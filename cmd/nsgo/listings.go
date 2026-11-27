package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const (
	storeListingSchemaVersion = 1
	defaultStoreListingsDir   = "store"

	storeTargetAppStore = "appstore"
	storeTargetPlay     = "play"

	// Official App Store Connect character and media limits.
	appStoreTitleMax           = 30
	appStoreSubtitleMax        = 30
	appStoreDescriptionMax     = 4000
	appStoreKeywordsMax        = 100
	appStoreReleaseNotesMax    = 4000
	appStorePromotionalTextMax = 170
	appStoreMaxScreenshots     = 10
	appStoreMaxPreviews        = 3

	// Official Google Play character and media limits.
	googlePlayTitleMax            = 30
	googlePlayShortDescriptionMax = 80
	googlePlayDescriptionMax      = 4000
	googlePlayReleaseNotesMax     = 500
	googlePlayMaxScreenshots      = 8
)

// storeManifest is the top-level store/listing.yaml file.
type storeManifest struct {
	Version       int                      `yaml:"version"`
	DefaultLocale string                   `yaml:"default_locale"`
	AppStore      appStoreManifestConfig   `yaml:"app_store"`
	GooglePlay    googlePlayManifestConfig `yaml:"google_play"`
}

type appStoreManifestConfig struct {
	BundleID          string `yaml:"bundle_id"`
	AppID             string `yaml:"app_id"`
	Platform          string `yaml:"platform"`
	Version           string `yaml:"version"`
	Copyright         string `yaml:"copyright"`
	PrimaryCategory   string `yaml:"primary_category"`
	SecondaryCategory string `yaml:"secondary_category"`
}

type googlePlayManifestConfig struct {
	PackageName     string `yaml:"package_name"`
	DefaultLanguage string `yaml:"default_language"`
	ContactEmail    string `yaml:"contact_email"`
	ContactWebsite  string `yaml:"contact_website"`
	ContactPhone    string `yaml:"contact_phone"`
	Track           string `yaml:"track"`
}

// storeKeywords accepts either a comma-separated string or a YAML list of keywords.
type storeKeywords struct {
	Value string
	Set   bool
}

func (k *storeKeywords) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var s string
		if err := node.Decode(&s); err != nil {
			return err
		}
		k.Value = strings.TrimSpace(s)
		k.Set = true
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := node.Decode(&list); err != nil {
			return err
		}
		cleaned := make([]string, 0, len(list))
		for i, item := range list {
			item = strings.TrimSpace(item)
			if item == "" {
				return fmt.Errorf("keywords[%d]: keyword must not be empty", i)
			}
			cleaned = append(cleaned, item)
		}
		k.Value = strings.Join(cleaned, ",")
		k.Set = true
		return nil
	default:
		return errors.New("keywords must be a string or list of strings")
	}
}

// localeListingFile is a per-language store/<locale>/listing.yaml file.
type localeListingFile struct {
	Version          int                      `yaml:"version"`
	Title            string                   `yaml:"title"`
	Subtitle         string                   `yaml:"subtitle"`
	ShortDescription string                   `yaml:"short_description"`
	Description      string                   `yaml:"description"`
	Keywords         storeKeywords            `yaml:"keywords"`
	ReleaseNotes     string                   `yaml:"release_notes"`
	PromotionalText  string                   `yaml:"promotional_text"`
	SupportURL       string                   `yaml:"support_url"`
	MarketingURL     string                   `yaml:"marketing_url"`
	PrivacyPolicyURL string                   `yaml:"privacy_policy_url"`
	VideoURL         string                   `yaml:"video_url"`
	AppStore         appStoreLocaleOverride   `yaml:"app_store"`
	GooglePlay       googlePlayLocaleOverride `yaml:"google_play"`
}

type appStoreLocaleOverride struct {
	Locale           string              `yaml:"locale"`
	Title            string              `yaml:"title"`
	Subtitle         string              `yaml:"subtitle"`
	Description      string              `yaml:"description"`
	Keywords         storeKeywords       `yaml:"keywords"`
	ReleaseNotes     string              `yaml:"release_notes"`
	PromotionalText  string              `yaml:"promotional_text"`
	SupportURL       string              `yaml:"support_url"`
	MarketingURL     string              `yaml:"marketing_url"`
	PrivacyPolicyURL string              `yaml:"privacy_policy_url"`
	Screenshots      map[string][]string `yaml:"screenshots"`
	Previews         map[string][]string `yaml:"previews"`
}

type googlePlayLocaleOverride struct {
	Locale           string              `yaml:"locale"`
	Title            string              `yaml:"title"`
	ShortDescription string              `yaml:"short_description"`
	Description      string              `yaml:"description"`
	ReleaseNotes     string              `yaml:"release_notes"`
	VideoURL         string              `yaml:"video_url"`
	Screenshots      map[string][]string `yaml:"screenshots"`
	FeatureGraphic   string              `yaml:"feature_graphic"`
	Icon             string              `yaml:"icon"`
}

// storeListings holds the parsed, resolved store metadata ready for validation and upload.
type storeListings struct {
	Dir      string
	Manifest storeManifest
	Locales  []resolvedLocaleListing
}

type resolvedLocaleListing struct {
	Locale     string // Directory name (e.g. "en-US", "zh-Hans")
	Dir        string
	AppStore   appStoreLocaleData
	GooglePlay googlePlayLocaleData
}

type appStoreLocaleData struct {
	Locale           string
	Title            string
	Subtitle         string
	Description      string
	Keywords         string
	ReleaseNotes     string
	PromotionalText  string
	SupportURL       string
	MarketingURL     string
	PrivacyPolicyURL string
	Screenshots      map[string][]string // display type -> absolute file paths
	Previews         map[string][]string // preview type -> absolute file paths
}

type googlePlayLocaleData struct {
	Locale           string
	Title            string
	ShortDescription string
	Description      string
	ReleaseNotes     string
	VideoURL         string
	Screenshots      map[string][]string // image type -> absolute file paths
	FeatureGraphic   string              // absolute file path
	Icon             string              // absolute file path
}

// storePublishOptions configures a store listing publish run.
type storePublishOptions struct {
	stores      []string
	listingsDir string
	dryRun      bool
	platform    string
	version     string
	track       string

	// Injectable API clients for testing.
	appStoreClient   *appStoreClient
	googlePlayClient *googlePlayClient
}

// publishStoreListings loads, validates, and uploads text-based store listings
// to App Store Connect and/or Google Play. Validation always completes before
// credentials are read or any upload HTTP request is sent.
func publishStoreListings(ctx context.Context, o storePublishOptions, path string) error {
	var p *project
	baseDir := path
	if baseDir == "" {
		baseDir = "."
	}
	loaded, err := load(baseDir)
	if err == nil {
		p = loaded
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	listingsDir := o.listingsDir
	if listingsDir != "" {
		if !filepath.IsAbs(listingsDir) && p != nil {
			candidate := p.resolve(listingsDir)
			if _, statErr := os.Stat(candidate); statErr == nil {
				listingsDir = candidate
			} else if _, cwdErr := os.Stat(listingsDir); cwdErr != nil {
				listingsDir = candidate
			}
		}
	} else if p != nil && p.Store.Dir != "" {
		listingsDir = p.Store.Dir
	} else if p != nil {
		listingsDir = p.resolve(defaultStoreListingsDir)
	} else {
		if _, err := os.Stat(filepath.Join(baseDir, "listing.yaml")); err == nil {
			listingsDir = baseDir
		} else {
			listingsDir = filepath.Join(baseDir, defaultStoreListingsDir)
		}
	}

	listings, err := loadStoreListings(listingsDir, p)
	if err != nil {
		return err
	}
	if o.platform != "" {
		listings.Manifest.AppStore.Platform = o.platform
	}
	if o.version != "" {
		listings.Manifest.AppStore.Version = o.version
	}
	if o.track != "" {
		listings.Manifest.GooglePlay.Track = o.track
	}

	if err := listings.validate(o.stores); err != nil {
		return err
	}

	if o.dryRun {
		done("validated %d store listing locale(s) for %s (%s)", len(listings.Locales), strings.Join(o.stores, ", "), rel(listings.Dir))
		return nil
	}

	for _, store := range o.stores {
		switch store {
		case storeTargetAppStore:
			if err := uploadAppStoreListings(ctx, listings, o.appStoreClient); err != nil {
				return err
			}
			done("uploaded %d locale(s) to App Store Connect (%s %s)", len(listings.Locales), listings.Manifest.AppStore.BundleID, listings.Manifest.AppStore.Version)
		case storeTargetPlay:
			if err := uploadGooglePlayListings(ctx, listings, o.googlePlayClient); err != nil {
				return err
			}
			done("uploaded %d locale(s) to Google Play (%s)", len(listings.Locales), listings.Manifest.GooglePlay.PackageName)
		}
	}
	return nil
}

// parseStoreTargets parses -store ("appstore", "play", "all", or comma-separated).
func parseStoreTargets(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{storeTargetAppStore, storeTargetPlay}, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, part := range splitList(raw) {
		name := strings.ToLower(part)
		switch name {
		case "all":
			for _, t := range []string{storeTargetAppStore, storeTargetPlay} {
				if !seen[t] {
					seen[t] = true
					out = append(out, t)
				}
			}
		case storeTargetAppStore, "apple", "ios", "macos":
			if !seen[storeTargetAppStore] {
				seen[storeTargetAppStore] = true
				out = append(out, storeTargetAppStore)
			}
		case storeTargetPlay, "googleplay", "google-play", "android":
			if !seen[storeTargetPlay] {
				seen[storeTargetPlay] = true
				out = append(out, storeTargetPlay)
			}
		default:
			return nil, fmt.Errorf("publish: unknown -store %q; use appstore, play or all", part)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("publish: -store must name appstore, play or all")
	}
	return out, nil
}

// loadStoreListings parses store/listing.yaml and every <locale>/listing.yaml
// (plus optional description.md, release_notes.md and promotional_text.md).
func loadStoreListings(dir string, p *project) (*storeListings, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(absDir, "listing.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("store listings: %w", err)
	}
	var manifest storeManifest
	if err := decodeStrictYAML(data, &manifest); err != nil {
		return nil, fmt.Errorf("%s: %w", rel(manifestPath), err)
	}
	if manifest.Version != storeListingSchemaVersion {
		return nil, fmt.Errorf("%s: version must be %d (got %d)", rel(manifestPath), storeListingSchemaVersion, manifest.Version)
	}
	// Apply defaults from nsgo.yaml when present.
	if p != nil {
		if manifest.AppStore.BundleID == "" {
			manifest.AppStore.BundleID = p.ID
		}
		if manifest.AppStore.Version == "" {
			manifest.AppStore.Version = p.Version
		}
		if manifest.AppStore.Copyright == "" {
			manifest.AppStore.Copyright = p.Copyright
		}
		if manifest.GooglePlay.PackageName == "" {
			manifest.GooglePlay.PackageName = p.ID
		}
	}
	if manifest.AppStore.Platform == "" {
		manifest.AppStore.Platform = "ios"
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("store listings: %w", err)
	}
	var locales []resolvedLocaleListing
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		locale := entry.Name()
		localeDir := filepath.Join(absDir, locale)
		localeYAMLPath := filepath.Join(localeDir, "listing.yaml")
		raw, err := os.ReadFile(localeYAMLPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("%s: missing listing.yaml in locale directory %q", rel(localeDir), locale)
			}
			return nil, fmt.Errorf("%s: %w", rel(localeYAMLPath), err)
		}
		var lf localeListingFile
		if err := decodeStrictYAML(raw, &lf); err != nil {
			return nil, fmt.Errorf("%s: %w", rel(localeYAMLPath), err)
		}
		if lf.Version != storeListingSchemaVersion {
			return nil, fmt.Errorf("%s: version must be %d (got %d)", rel(localeYAMLPath), storeListingSchemaVersion, lf.Version)
		}
		desc, err := loadMarkdownOrYAML(localeDir, "description.md", "description", lf.Description)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", locale, err)
		}
		notes, err := loadMarkdownOrYAML(localeDir, "release_notes.md", "release_notes", lf.ReleaseNotes)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", locale, err)
		}
		promo, err := loadMarkdownOrYAML(localeDir, "promotional_text.md", "promotional_text", lf.PromotionalText)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", locale, err)
		}

		ascLocale := strings.TrimSpace(firstNonEmpty(lf.AppStore.Locale, mapToAppStoreLocale(locale)))
		playLocale := strings.TrimSpace(firstNonEmpty(lf.GooglePlay.Locale, mapToGooglePlayLocale(locale)))

		ascKeywords := lf.Keywords.Value
		if lf.AppStore.Keywords.Set {
			ascKeywords = lf.AppStore.Keywords.Value
		}

		ascScreenshots, err := resolveMediaMap(absDir, localeDir, lf.AppStore.Screenshots)
		if err != nil {
			return nil, fmt.Errorf("%s/listing.yaml: app_store.screenshots: %w", locale, err)
		}
		ascPreviews, err := resolveMediaMap(absDir, localeDir, lf.AppStore.Previews)
		if err != nil {
			return nil, fmt.Errorf("%s/listing.yaml: app_store.previews: %w", locale, err)
		}
		playScreenshots, err := resolveMediaMap(absDir, localeDir, lf.GooglePlay.Screenshots)
		if err != nil {
			return nil, fmt.Errorf("%s/listing.yaml: google_play.screenshots: %w", locale, err)
		}
		featureGraphic, err := resolveOptionalMediaPath(absDir, localeDir, lf.GooglePlay.FeatureGraphic)
		if err != nil {
			return nil, fmt.Errorf("%s/listing.yaml: google_play.feature_graphic: %w", locale, err)
		}
		playIcon, err := resolveOptionalMediaPath(absDir, localeDir, lf.GooglePlay.Icon)
		if err != nil {
			return nil, fmt.Errorf("%s/listing.yaml: google_play.icon: %w", locale, err)
		}

		locales = append(locales, resolvedLocaleListing{
			Locale: locale,
			Dir:    localeDir,
			AppStore: appStoreLocaleData{
				Locale:           ascLocale,
				Title:            normalizeListingText(firstNonEmpty(lf.AppStore.Title, lf.Title)),
				Subtitle:         normalizeListingText(firstNonEmpty(lf.AppStore.Subtitle, lf.Subtitle)),
				Description:      normalizeListingText(firstNonEmpty(lf.AppStore.Description, desc)),
				Keywords:         strings.TrimSpace(ascKeywords),
				ReleaseNotes:     normalizeListingText(firstNonEmpty(lf.AppStore.ReleaseNotes, notes)),
				PromotionalText:  normalizeListingText(firstNonEmpty(lf.AppStore.PromotionalText, promo)),
				SupportURL:       strings.TrimSpace(firstNonEmpty(lf.AppStore.SupportURL, lf.SupportURL)),
				MarketingURL:     strings.TrimSpace(firstNonEmpty(lf.AppStore.MarketingURL, lf.MarketingURL)),
				PrivacyPolicyURL: strings.TrimSpace(firstNonEmpty(lf.AppStore.PrivacyPolicyURL, lf.PrivacyPolicyURL)),
				Screenshots:      ascScreenshots,
				Previews:         ascPreviews,
			},
			GooglePlay: googlePlayLocaleData{
				Locale:           playLocale,
				Title:            normalizeListingText(firstNonEmpty(lf.GooglePlay.Title, lf.Title)),
				ShortDescription: normalizeListingText(firstNonEmpty(lf.GooglePlay.ShortDescription, lf.ShortDescription)),
				Description:      normalizeListingText(firstNonEmpty(lf.GooglePlay.Description, desc)),
				ReleaseNotes:     normalizeListingText(firstNonEmpty(lf.GooglePlay.ReleaseNotes, notes)),
				VideoURL:         strings.TrimSpace(firstNonEmpty(lf.GooglePlay.VideoURL, lf.VideoURL)),
				Screenshots:      playScreenshots,
				FeatureGraphic:   featureGraphic,
				Icon:             playIcon,
			},
		})
	}
	sort.Slice(locales, func(i, j int) bool { return locales[i].Locale < locales[j].Locale })
	return &storeListings{
		Dir:      absDir,
		Manifest: manifest,
		Locales:  locales,
	}, nil
}

// validate checks every store listing rule (schema, locales, required fields,
// character limits, URLs, and media files) before any upload request is sent.
func (l *storeListings) validate(targets []string) error {
	var errs []string
	addErr := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	if len(l.Locales) == 0 {
		addErr("listing.yaml: no locale directories found in %s", rel(l.Dir))
	}
	if l.Manifest.DefaultLocale != "" {
		found := false
		for _, loc := range l.Locales {
			if loc.Locale == l.Manifest.DefaultLocale {
				found = true
				break
			}
		}
		if !found {
			addErr("listing.yaml: default_locale %q has no matching locale directory", l.Manifest.DefaultLocale)
		}
	}

	wantAppStore := slices.Contains(targets, storeTargetAppStore)
	wantPlay := slices.Contains(targets, storeTargetPlay)

	if wantAppStore {
		if strings.TrimSpace(l.Manifest.AppStore.BundleID) == "" && strings.TrimSpace(l.Manifest.AppStore.AppID) == "" {
			addErr("listing.yaml: app_store.bundle_id (or app_store.app_id) is required")
		}
		if _, ok := normalizeAppStorePlatform(l.Manifest.AppStore.Platform); !ok {
			addErr("listing.yaml: app_store.platform %q is invalid; use ios, macos, tvos or visionos", l.Manifest.AppStore.Platform)
		}
		if strings.TrimSpace(l.Manifest.AppStore.Version) == "" {
			addErr("listing.yaml: app_store.version is required")
		}

		seenASCLocales := map[string]string{}
		for _, loc := range l.Locales {
			a := loc.AppStore
			prefix := loc.Locale + "/listing.yaml"
			if !validAppStoreLocales[a.Locale] {
				addErr("%s: unsupported App Store locale %q", prefix, a.Locale)
			} else if prev, dup := seenASCLocales[a.Locale]; dup {
				addErr("%s: App Store locale %q duplicates %s", prefix, a.Locale, prev)
			} else {
				seenASCLocales[a.Locale] = loc.Locale
			}

			validateRequiredLen(&errs, prefix, "app_store title", a.Title, appStoreTitleMax)
			validateOptionalLen(&errs, prefix, "app_store subtitle", a.Subtitle, appStoreSubtitleMax)
			validateRequiredLen(&errs, prefix, "app_store description", a.Description, appStoreDescriptionMax)
			validateOptionalLen(&errs, prefix, "app_store keywords", a.Keywords, appStoreKeywordsMax)
			validateOptionalLen(&errs, prefix, "app_store release_notes", a.ReleaseNotes, appStoreReleaseNotesMax)
			validateOptionalLen(&errs, prefix, "app_store promotional_text", a.PromotionalText, appStorePromotionalTextMax)

			if a.SupportURL == "" {
				addErr("%s: app_store support_url is required", prefix)
			} else if err := validateHTTPURL(a.SupportURL); err != nil {
				addErr("%s: app_store support_url: %v", prefix, err)
			}
			if a.MarketingURL != "" {
				if err := validateHTTPURL(a.MarketingURL); err != nil {
					addErr("%s: app_store marketing_url: %v", prefix, err)
				}
			}
			if a.PrivacyPolicyURL != "" {
				if err := validateHTTPURL(a.PrivacyPolicyURL); err != nil {
					addErr("%s: app_store privacy_policy_url: %v", prefix, err)
				}
			}

			for _, displayType := range sortedMapKeys(a.Screenshots) {
				files := a.Screenshots[displayType]
				if !validAppStoreScreenshotTypes[displayType] {
					addErr("%s: unsupported App Store screenshot display type %q", prefix, displayType)
				}
				if len(files) > appStoreMaxScreenshots {
					addErr("%s: app_store screenshots[%s] has %d files (max %d)", prefix, displayType, len(files), appStoreMaxScreenshots)
				}
				for _, f := range files {
					if err := validateMediaFile(f, []string{".png", ".jpg", ".jpeg"}); err != nil {
						addErr("%s: app_store screenshots[%s]: %v", prefix, displayType, err)
					}
				}
			}
			for _, previewType := range sortedMapKeys(a.Previews) {
				files := a.Previews[previewType]
				if !validAppStorePreviewTypes[previewType] {
					addErr("%s: unsupported App Store preview type %q", prefix, previewType)
				}
				if len(files) > appStoreMaxPreviews {
					addErr("%s: app_store previews[%s] has %d files (max %d)", prefix, previewType, len(files), appStoreMaxPreviews)
				}
				for _, f := range files {
					if err := validateMediaFile(f, []string{".mp4", ".mov", ".m4v"}); err != nil {
						addErr("%s: app_store previews[%s]: %v", prefix, previewType, err)
					}
				}
			}
		}
	}

	if wantPlay {
		if strings.TrimSpace(l.Manifest.GooglePlay.PackageName) == "" {
			addErr("listing.yaml: google_play.package_name is required")
		}
		if l.Manifest.GooglePlay.ContactWebsite != "" {
			if err := validateHTTPURL(l.Manifest.GooglePlay.ContactWebsite); err != nil {
				addErr("listing.yaml: google_play.contact_website: %v", err)
			}
		}
		if l.Manifest.GooglePlay.DefaultLanguage != "" {
			mapped := mapToGooglePlayLocale(l.Manifest.GooglePlay.DefaultLanguage)
			if !validGooglePlayLocales[mapped] {
				addErr("listing.yaml: unsupported Google Play default_language %q", l.Manifest.GooglePlay.DefaultLanguage)
			}
		}
		if l.Manifest.GooglePlay.Track != "" {
			if strings.ContainsAny(l.Manifest.GooglePlay.Track, "/ \t\n\r") {
				addErr("listing.yaml: google_play.track %q contains invalid characters", l.Manifest.GooglePlay.Track)
			}
		}

		seenPlayLocales := map[string]string{}
		for _, loc := range l.Locales {
			g := loc.GooglePlay
			prefix := loc.Locale + "/listing.yaml"
			if !validGooglePlayLocales[g.Locale] {
				addErr("%s: unsupported Google Play locale %q", prefix, g.Locale)
			} else if prev, dup := seenPlayLocales[g.Locale]; dup {
				addErr("%s: Google Play locale %q duplicates %s", prefix, g.Locale, prev)
			} else {
				seenPlayLocales[g.Locale] = loc.Locale
			}

			validateRequiredLen(&errs, prefix, "google_play title", g.Title, googlePlayTitleMax)
			validateRequiredLen(&errs, prefix, "google_play short_description", g.ShortDescription, googlePlayShortDescriptionMax)
			validateRequiredLen(&errs, prefix, "google_play description", g.Description, googlePlayDescriptionMax)
			validateOptionalLen(&errs, prefix, "google_play release_notes", g.ReleaseNotes, googlePlayReleaseNotesMax)

			if g.VideoURL != "" {
				if err := validateHTTPURL(g.VideoURL); err != nil {
					addErr("%s: google_play video_url: %v", prefix, err)
				}
			}
			for _, imgType := range sortedMapKeys(g.Screenshots) {
				files := g.Screenshots[imgType]
				if !validGooglePlayScreenshotTypes[imgType] {
					addErr("%s: unsupported Google Play screenshot type %q", prefix, imgType)
				}
				if len(files) > googlePlayMaxScreenshots {
					addErr("%s: google_play screenshots[%s] has %d files (max %d)", prefix, imgType, len(files), googlePlayMaxScreenshots)
				}
				for _, f := range files {
					if err := validateMediaFile(f, []string{".png", ".jpg", ".jpeg"}); err != nil {
						addErr("%s: google_play screenshots[%s]: %v", prefix, imgType, err)
					}
				}
			}
			if g.FeatureGraphic != "" {
				if err := validateMediaFile(g.FeatureGraphic, []string{".png", ".jpg", ".jpeg"}); err != nil {
					addErr("%s: google_play feature_graphic: %v", prefix, err)
				}
			}
			if g.Icon != "" {
				if err := validateMediaFile(g.Icon, []string{".png"}); err != nil {
					addErr("%s: google_play icon: %v", prefix, err)
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("store listings validation failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

func validateRequiredLen(errs *[]string, prefix, field, value string, max int) {
	if value == "" {
		*errs = append(*errs, fmt.Sprintf("%s: %s is required", prefix, field))
		return
	}
	if n := utf8.RuneCountInString(value); n > max {
		*errs = append(*errs, fmt.Sprintf("%s: %s is %d characters (max %d)", prefix, field, n, max))
	}
}

func validateOptionalLen(errs *[]string, prefix, field, value string, max int) {
	if n := utf8.RuneCountInString(value); n > max {
		*errs = append(*errs, fmt.Sprintf("%s: %s is %d characters (max %d)", prefix, field, n, max))
	}
}

func validateHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("URL %q must use http or https with a host", raw)
	}
	return nil
}

func validateMediaFile(absPath string, allowedExts []string) error {
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("%s: %w", rel(absPath), err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, expected a file", rel(absPath))
	}
	if info.Size() == 0 {
		return fmt.Errorf("%s is empty", rel(absPath))
	}
	ext := strings.ToLower(filepath.Ext(absPath))
	if !slices.Contains(allowedExts, ext) {
		return fmt.Errorf("%s has unsupported extension %q (allowed: %s)", rel(absPath), ext, strings.Join(allowedExts, ", "))
	}
	return nil
}

func decodeStrictYAML(data []byte, dst any) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("empty YAML file")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple YAML documents are not allowed")
		}
		return err
	}
	return nil
}

func loadMarkdownOrYAML(localeDir, mdFile, fieldName, yamlValue string) (string, error) {
	yamlTrimmed := normalizeListingText(yamlValue)
	mdPath := filepath.Join(localeDir, mdFile)
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return yamlTrimmed, nil
		}
		return "", fmt.Errorf("%s: %w", mdFile, err)
	}
	mdTrimmed := normalizeListingText(string(mdBytes))
	if yamlTrimmed != "" && mdTrimmed != "" {
		return "", fmt.Errorf("cannot specify both %s in listing.yaml and %s", fieldName, mdFile)
	}
	if mdTrimmed != "" {
		return mdTrimmed, nil
	}
	return yamlTrimmed, nil
}

func normalizeListingText(s string) string {
	s = strings.TrimPrefix(s, "\uFEFF")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimSpace(s)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func resolveMediaMap(rootDir, localeDir string, raw map[string][]string) (map[string][]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string][]string, len(raw))
	for _, k := range sortedMapKeys(raw) {
		paths := raw[k]
		resolved := make([]string, 0, len(paths))
		for _, p := range paths {
			abs, err := resolveSafeMediaPath(rootDir, localeDir, p)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			resolved = append(resolved, abs)
		}
		out[k] = resolved
	}
	return out, nil
}

func resolveOptionalMediaPath(rootDir, localeDir, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return resolveSafeMediaPath(rootDir, localeDir, raw)
}

func resolveSafeMediaPath(rootDir, localeDir, p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errors.New("media path must not be empty")
	}
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("media path %q must be relative to the locale directory", p)
	}
	joined := filepath.Clean(filepath.Join(localeDir, filepath.FromSlash(p)))
	relToRoot, err := filepath.Rel(rootDir, joined)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("media path %q escapes the listings directory", p)
	}
	return joined, nil
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func normalizeAppStorePlatform(p string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "", "ios":
		return "IOS", true
	case "macos", "mac_os", "osx":
		return "MAC_OS", true
	case "tvos", "tv_os":
		return "TV_OS", true
	case "visionos", "vision_os":
		return "VISION_OS", true
	default:
		return "", false
	}
}

// mapToAppStoreLocale maps common BCP-47 / Google Play locale codes to App Store Connect locale codes.
func mapToAppStoreLocale(locale string) string {
	switch locale {
	case "zh-CN", "zh-Hans-CN":
		return "zh-Hans"
	case "zh-TW", "zh-HK", "zh-Hant-TW", "zh-Hant-HK":
		return "zh-Hant"
	case "ja-JP":
		return "ja"
	case "ko-KR":
		return "ko"
	case "it-IT":
		return "it"
	case "ar":
		return "ar-SA"
	case "hi-IN":
		return "hi"
	case "iw-IL", "he-IL":
		return "he"
	case "da-DK":
		return "da"
	case "fi-FI":
		return "fi"
	case "nb-NO", "no-NO":
		return "no"
	case "cs-CZ":
		return "cs"
	case "el-GR":
		return "el"
	case "hu-HU":
		return "hu"
	case "ro-RO":
		return "ro"
	case "sk-SK":
		return "sk"
	case "hr-HR":
		return "hr"
	case "ca-ES":
		return "ca"
	case "pl-PL":
		return "pl"
	case "ru-RU":
		return "ru"
	case "sv-SE":
		return "sv"
	case "tr-TR":
		return "tr"
	case "uk-UA":
		return "uk"
	case "vi-VN":
		return "vi"
	case "th-TH":
		return "th"
	case "id-ID":
		return "id"
	case "ms-MY":
		return "ms"
	default:
		return locale
	}
}

// mapToGooglePlayLocale maps common BCP-47 / App Store locale codes to Google Play locale codes.
func mapToGooglePlayLocale(locale string) string {
	switch locale {
	case "zh-Hans", "zh-Hans-CN":
		return "zh-CN"
	case "zh-Hant", "zh-Hant-TW":
		return "zh-TW"
	case "zh-Hant-HK":
		return "zh-HK"
	case "ja":
		return "ja-JP"
	case "ko":
		return "ko-KR"
	case "it":
		return "it-IT"
	case "ar-SA":
		return "ar"
	case "hi":
		return "hi-IN"
	case "he", "he-IL":
		return "iw-IL"
	case "da":
		return "da-DK"
	case "fi":
		return "fi-FI"
	case "no":
		return "no-NO"
	case "cs":
		return "cs-CZ"
	case "el":
		return "el-GR"
	case "hu":
		return "hu-HU"
	case "pl":
		return "pl-PL"
	case "ru":
		return "ru-RU"
	case "sv":
		return "sv-SE"
	case "tr":
		return "tr-TR"
	case "es-MX":
		return "es-419"
	default:
		return locale
	}
}

// Official App Store Connect localization locale codes.
var validAppStoreLocales = map[string]bool{
	"ar-SA": true, "ca": true, "cs": true, "da": true, "de-DE": true,
	"el": true, "en-AU": true, "en-CA": true, "en-GB": true, "en-US": true,
	"es-ES": true, "es-MX": true, "fi": true, "fr-CA": true, "fr-FR": true,
	"he": true, "hi": true, "hr": true, "hu": true, "id": true,
	"it": true, "ja": true, "ko": true, "ms": true, "nl-NL": true,
	"no": true, "pl": true, "pt-BR": true, "pt-PT": true, "ro": true,
	"ru": true, "sk": true, "sv": true, "th": true, "tr": true,
	"uk": true, "vi": true, "zh-Hans": true, "zh-Hant": true,
}

// Official Google Play Console store listing locale codes.
var validGooglePlayLocales = map[string]bool{
	"af": true, "am": true, "ar": true, "az-AZ": true, "be": true,
	"bg": true, "bn-BD": true, "ca": true, "cs-CZ": true, "da-DK": true,
	"de-DE": true, "el-GR": true, "en-AU": true, "en-CA": true, "en-GB": true,
	"en-IN": true, "en-SG": true, "en-US": true, "en-ZA": true, "es-419": true,
	"es-ES": true, "es-US": true, "et": true, "eu-ES": true, "fa": true,
	"fa-AE": true, "fa-AF": true, "fa-IR": true, "fi-FI": true, "fil": true,
	"fr-CA": true, "fr-FR": true, "gl-ES": true, "gu": true, "hi-IN": true,
	"hr": true, "hu-HU": true, "hy-AM": true, "id": true, "is-IS": true,
	"it-IT": true, "iw-IL": true, "ja-JP": true, "ka-GE": true, "kk": true,
	"km-KH": true, "kn-IN": true, "ko-KR": true, "ky-KG": true, "lo-LA": true,
	"lt": true, "lv": true, "mk-MK": true, "ml-IN": true, "mn-MN": true,
	"mr-IN": true, "ms": true, "ms-MY": true, "my-MM": true, "ne-NP": true,
	"nl-NL": true, "no-NO": true, "pa": true, "pl-PL": true, "pt-BR": true,
	"pt-PT": true, "rm": true, "ro": true, "ru-RU": true, "si-LK": true,
	"sk": true, "sl": true, "sq": true, "sr": true, "sv-SE": true,
	"sw": true, "ta-IN": true, "te-IN": true, "th": true, "tr-TR": true,
	"uk": true, "ur": true, "vi": true, "zh-CN": true, "zh-HK": true,
	"zh-TW": true, "zu": true,
}

// Official App Store Connect screenshotDisplayType values.
var validAppStoreScreenshotTypes = map[string]bool{
	"APP_IPHONE_67": true, "APP_IPHONE_65": true, "APP_IPHONE_61": true,
	"APP_IPHONE_58": true, "APP_IPHONE_55": true, "APP_IPHONE_47": true,
	"APP_IPHONE_40": true, "APP_IPHONE_35": true,
	"APP_IPAD_PRO_3GEN_129": true, "APP_IPAD_PRO_3GEN_11": true,
	"APP_IPAD_PRO_129": true, "APP_IPAD_105": true, "APP_IPAD_97": true,
	"APP_DESKTOP": true, "APP_WATCH_ULTRA": true, "APP_WATCH_SERIES_10": true,
	"APP_WATCH_SERIES_7": true, "APP_WATCH_SERIES_4": true, "APP_WATCH_SERIES_3": true,
	"APP_APPLE_TV": true, "APP_APPLE_VISION_PRO": true,
	"IMESSAGE_APP_IPHONE_67": true, "IMESSAGE_APP_IPHONE_65": true,
	"IMESSAGE_APP_IPHONE_61": true, "IMESSAGE_APP_IPHONE_58": true,
	"IMESSAGE_APP_IPHONE_55": true, "IMESSAGE_APP_IPAD_PRO_3GEN_129": true,
	"IMESSAGE_APP_IPAD_PRO_3GEN_11": true, "IMESSAGE_APP_IPAD_PRO_129": true,
}

// Official App Store Connect previewType values.
var validAppStorePreviewTypes = map[string]bool{
	"IPHONE_67": true, "IPHONE_65": true, "IPHONE_61": true,
	"IPHONE_58": true, "IPHONE_55": true, "IPHONE_47": true,
	"IPHONE_40": true, "IPHONE_35": true,
	"IPAD_PRO_3GEN_129": true, "IPAD_PRO_3GEN_11": true,
	"IPAD_PRO_129": true, "IPAD_105": true, "IPAD_97": true,
	"DESKTOP": true, "APPLE_TV": true, "APPLE_VISION_PRO": true,
}

// Official Google Play Android Publisher API screenshot imageType values.
var validGooglePlayScreenshotTypes = map[string]bool{
	"phoneScreenshots":     true,
	"sevenInchScreenshots": true,
	"tenInchScreenshots":   true,
	"tvScreenshots":        true,
	"wearScreenshots":      true,
}
