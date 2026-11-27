package main

import (
	"bytes"
	"cmp"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
	"golang.org/x/mod/module"
)

// NSGo's built-in library and component index (#2461). Bare names given to nsgo add resolve only in this embedded YAML file, so what a name means changes only through a reviewed pull request and a release, covered by nsgo's own go.sum and checksum database entries, with nothing fetched or cached.

//go:embed registry.yaml
var builtinRegistryYAML []byte

const (
	registryIndexVersion      = 1
	maxRegistryIndexBytes     = 4 << 20 // 4 MiB
	maxRegistryYAMLDepth      = 32
	maxRegistryNameLen        = 64
	maxRegistryDescriptionLen = 200
)

// registryIndex is version 1 of the YAML index of NSGo libraries and components.
type registryIndex struct {
	Version int            `yaml:"version"`
	Items   []registryItem `yaml:"items"`
}

// registryItem is one library or component listed in a registry index.
type registryItem struct {
	Name        string   `yaml:"name"`
	Module      string   `yaml:"module"`
	Component   string   `yaml:"component,omitempty"`
	Version     string   `yaml:"version,omitempty"`
	Description string   `yaml:"description"`
	License     string   `yaml:"license"`
	Homepage    string   `yaml:"homepage,omitempty"`
	Tags        []string `yaml:"tags,omitempty,flow"`
}

// kind returns "component" when the item declares a component name, else "library".
func (item registryItem) kind() string {
	if item.Component != "" {
		return "component"
	}
	return "library"
}

// builtinRegistry returns the parsed embedded registry.yaml.
var builtinRegistry = sync.OnceValue(func() *registryIndex {
	idx, err := parseRegistryIndex(builtinRegistryYAML)
	if err != nil {
		panic("cmd/nsgo: invalid embedded registry.yaml: " + err.Error())
	}
	return idx
})

// lookup finds the item with the given bare name.
func (idx *registryIndex) lookup(name string) (registryItem, bool) {
	if idx == nil {
		return registryItem{}, false
	}
	for _, item := range idx.Items {
		if item.Name == name {
			return item, true
		}
	}
	return registryItem{}, false
}

// list returns the items whose name, description, or tags contain every word, case-insensitively.
func (idx *registryIndex) list(words []string) []registryItem {
	if idx == nil || len(idx.Items) == 0 {
		return nil
	}
	needles := make([]string, len(words))
	for i, w := range words {
		needles[i] = strings.ToLower(w)
	}
	var matched []registryItem
	for _, item := range idx.Items {
		if item.matchesWords(needles) {
			matched = append(matched, item)
		}
	}
	return matched
}

func (item registryItem) matchesWords(needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	name := strings.ToLower(item.Name)
	desc := strings.ToLower(item.Description)
	for _, needle := range needles {
		if strings.Contains(name, needle) || strings.Contains(desc, needle) {
			continue
		}
		found := false
		for _, tag := range item.Tags {
			if strings.Contains(strings.ToLower(tag), needle) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// listRegistryItems writes the matching items' names, kinds, modules, and descriptions to out.
func listRegistryItems(idx *registryIndex, words []string, out io.Writer) error {
	items := idx.list(words)
	if len(items) == 0 {
		return nil
	}
	var nameWidth, kindWidth, modWidth int
	for _, item := range items {
		nameWidth = max(nameWidth, len(item.Name))
		kindWidth = max(kindWidth, len(item.kind()))
		modWidth = max(modWidth, len(item.Module))
	}
	var b bytes.Buffer
	for _, item := range items {
		fmt.Fprintf(&b, "%-*s  %-*s  %-*s  %s\n", nameWidth, item.Name, kindWidth, item.kind(), modWidth, item.Module, item.Description)
	}
	_, err := out.Write(b.Bytes())
	return err
}

// parseRegistryIndex parses and validates a version 1 YAML registry index.
func parseRegistryIndex(data []byte) (*registryIndex, error) {
	if len(data) > maxRegistryIndexBytes {
		return nil, fmt.Errorf("registry index exceeds %d bytes", maxRegistryIndexBytes)
	}
	if !utf8.Valid(data) {
		return nil, errors.New("registry index is not valid UTF-8")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("empty YAML file")
	}
	if err := inspectRegistryYAML(data); err != nil {
		return nil, err
	}
	var idx registryIndex
	if err := decodeStrictYAML(data, &idx); err != nil {
		return nil, err
	}
	if idx.Items == nil {
		idx.Items = []registryItem{}
	}
	for i := range idx.Items {
		if len(idx.Items[i].Tags) == 0 {
			idx.Items[i].Tags = nil
		}
	}
	if err := validateRegistryIndex(&idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// inspectRegistryYAML parses the YAML AST before struct decoding so anchors, aliases, merge keys, multiple documents, duplicate keys, and control or format characters are rejected before any alias expansion can occur.
func inspectRegistryYAML(data []byte) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple YAML documents are not allowed")
		}
		return err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return errors.New("registry index must be a YAML mapping")
	}
	return walkRegistryYAMLNode(&doc, 0)
}

func walkRegistryYAMLNode(n *yaml.Node, depth int) error {
	if depth > maxRegistryYAMLDepth {
		return errors.New("YAML nesting depth limit exceeded")
	}
	if n.Anchor != "" {
		return fmt.Errorf("line %d: YAML anchors are not allowed", n.Line)
	}
	if n.Kind == yaml.AliasNode || n.Alias != nil {
		return fmt.Errorf("line %d: YAML aliases are not allowed", n.Line)
	}
	if n.Tag == "!!merge" {
		return fmt.Errorf("line %d: YAML merge keys are not allowed", n.Line)
	}
	for _, comment := range []string{n.HeadComment, n.LineComment, n.FootComment} {
		if err := validateNoFormatChars(comment); err != nil {
			return fmt.Errorf("line %d: %w", n.Line, err)
		}
	}
	if n.Kind == yaml.ScalarNode {
		if err := validateNoControlOrFormat(n.Value); err != nil {
			return fmt.Errorf("line %d: %w", n.Line, err)
		}
	}
	if n.Kind == yaml.MappingNode {
		seen := make(map[string]bool, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Tag == "!!merge" {
				return fmt.Errorf("line %d: YAML merge keys are not allowed", k.Line)
			}
			if k.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: YAML mapping keys must be scalars", k.Line)
			}
			if seen[k.Value] {
				return fmt.Errorf("line %d: duplicate YAML key %q", k.Line, k.Value)
			}
			seen[k.Value] = true
		}
	}
	for _, child := range n.Content {
		if err := walkRegistryYAMLNode(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validateNoFormatChars(s string) error {
	for _, r := range s {
		if unicode.In(r, unicode.Cf) {
			return fmt.Errorf("disallowed Unicode format character U+%04X", r)
		}
	}
	return nil
}

func validateNoControlOrFormat(s string) error {
	if !utf8.ValidString(s) {
		return errors.New("invalid UTF-8")
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return fmt.Errorf("disallowed control or format character U+%04X", r)
		}
	}
	return nil
}

func validateRegistryIndex(idx *registryIndex) error {
	if idx.Version != registryIndexVersion {
		return fmt.Errorf("unsupported registry index version %d; expected %d", idx.Version, registryIndexVersion)
	}
	seenNames := make(map[string]bool, len(idx.Items))
	for i, item := range idx.Items {
		if !isValidRegistryName(item.Name) {
			return fmt.Errorf("item %d: invalid name %q: must be 1 to %d characters of [a-z0-9-] without leading or trailing '-'", i, item.Name, maxRegistryNameLen)
		}
		if seenNames[item.Name] {
			return fmt.Errorf("duplicate item name %q", item.Name)
		}
		seenNames[item.Name] = true

		if err := module.CheckPath(item.Module); err != nil {
			return fmt.Errorf("item %q: invalid module %q: %w", item.Name, item.Module, err)
		}
		if item.Component != "" && !isValidRegistryName(item.Component) {
			return fmt.Errorf("item %q: invalid component %q: must be 1 to %d characters of [a-z0-9-] without leading or trailing '-'", item.Name, item.Component, maxRegistryNameLen)
		}
		if item.Version != "" {
			if module.IsPseudoVersion(item.Version) {
				return fmt.Errorf("item %q: version %q must be a release tag, not a pseudo-version", item.Name, item.Version)
			}
			if module.CanonicalVersion(item.Version) != item.Version {
				return fmt.Errorf("item %q: invalid version %q: must be a canonical release tag", item.Name, item.Version)
			}
			if err := module.Check(item.Module, item.Version); err != nil {
				return fmt.Errorf("item %q: invalid version %q: %w", item.Name, item.Version, err)
			}
		}
		if err := validateNoControlOrFormat(item.Description); err != nil {
			return fmt.Errorf("item %q: description: %w", item.Name, err)
		}
		if strings.TrimSpace(item.Description) == "" {
			return fmt.Errorf("item %q: description is required", item.Name)
		}
		if strings.TrimSpace(item.Description) != item.Description {
			return fmt.Errorf("item %q: description must not have leading or trailing whitespace", item.Name)
		}
		if n := utf8.RuneCountInString(item.Description); n > maxRegistryDescriptionLen {
			return fmt.Errorf("item %q: description is %d characters (max %d)", item.Name, n, maxRegistryDescriptionLen)
		}
		if err := validateSPDXExpression(item.License); err != nil {
			return fmt.Errorf("item %q: invalid license %q: %w", item.Name, item.License, err)
		}
		if item.Homepage != "" {
			if err := validateRegistryHomepage(item.Homepage); err != nil {
				return fmt.Errorf("item %q: invalid homepage %q: %w", item.Name, item.Homepage, err)
			}
		}
		seenTags := make(map[string]bool, len(item.Tags))
		for _, tag := range item.Tags {
			if !isValidRegistryName(tag) {
				return fmt.Errorf("item %q: invalid tag %q: must be a lower-case word of [a-z0-9-] without leading or trailing '-'", item.Name, tag)
			}
			if seenTags[tag] {
				return fmt.Errorf("item %q: duplicate tag %q", item.Name, tag)
			}
			seenTags[tag] = true
		}
	}
	return nil
}

// isValidRegistryName reports whether s is 1 to 64 characters of [a-z0-9-], not starting or ending with '-'.
func isValidRegistryName(s string) bool {
	if len(s) == 0 || len(s) > maxRegistryNameLen {
		return false
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

func validateRegistryHomepage(raw string) error {
	if err := validateNoControlOrFormat(raw); err != nil {
		return err
	}
	if strings.ContainsAny(raw, " \t\r\n") {
		return errors.New("URL must not contain whitespace")
	}
	if !strings.HasPrefix(raw, "https://") {
		return errors.New("URL must use https")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" || u.Hostname() == "" {
		return errors.New("URL must use https with a host")
	}
	if u.User != nil {
		return errors.New("URL must not contain user info")
	}
	return nil
}

// validateSPDXExpression validates an SPDX license expression according to SPDX Annex D grammar: compound expressions with AND, OR, WITH, parentheses, LicenseRef- / DocumentRef-, and + suffixes on license identifiers.
func validateSPDXExpression(expr string) error {
	if expr == "" {
		return errors.New("license is required")
	}
	if err := validateNoControlOrFormat(expr); err != nil {
		return err
	}
	if strings.TrimSpace(expr) != expr {
		return errors.New("license must not have leading or trailing whitespace")
	}
	tokens, err := tokenizeSPDX(expr)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		return errors.New("license is required")
	}
	p := spdxParser{tokens: tokens}
	if err := p.parseOr(0); err != nil {
		return err
	}
	if p.pos != len(p.tokens) {
		return fmt.Errorf("unexpected token %q", p.tokens[p.pos])
	}
	return nil
}

func tokenizeSPDX(expr string) ([]string, error) {
	var tokens []string
	i := 0
	for i < len(expr) {
		c := expr[i]
		switch c {
		case ' ':
			i++
		case '(', ')':
			tokens = append(tokens, string(c))
			i++
		default:
			if c > 0x7f {
				return nil, fmt.Errorf("non-ASCII character %q in SPDX expression", c)
			}
			start := i
			for i < len(expr) && expr[i] != ' ' && expr[i] != '(' && expr[i] != ')' {
				if expr[i] > 0x7f {
					return nil, fmt.Errorf("non-ASCII character in SPDX expression")
				}
				i++
			}
			tokens = append(tokens, expr[start:i])
		}
	}
	return tokens, nil
}

type spdxParser struct {
	tokens []string
	pos    int
}

func (p *spdxParser) peek() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	return p.tokens[p.pos]
}

func (p *spdxParser) parseOr(depth int) error {
	if err := p.parseAnd(depth); err != nil {
		return err
	}
	for p.peek() == "OR" {
		p.pos++
		if err := p.parseAnd(depth); err != nil {
			return err
		}
	}
	return nil
}

func (p *spdxParser) parseAnd(depth int) error {
	if err := p.parseFactor(depth); err != nil {
		return err
	}
	for p.peek() == "AND" {
		p.pos++
		if err := p.parseFactor(depth); err != nil {
			return err
		}
	}
	return nil
}

func (p *spdxParser) parseFactor(depth int) error {
	if depth > maxRegistryYAMLDepth {
		return errors.New("SPDX expression nesting depth limit exceeded")
	}
	tok := p.peek()
	if tok == "" {
		return errors.New("unexpected end of SPDX expression")
	}
	if tok == "(" {
		p.pos++
		if err := p.parseOr(depth + 1); err != nil {
			return err
		}
		if p.peek() != ")" {
			return errors.New("missing closing ')' in SPDX expression")
		}
		p.pos++
		return nil
	}
	if err := p.parseSimple(); err != nil {
		return err
	}
	if p.peek() == "WITH" {
		p.pos++
		exc := p.peek()
		if exc == "" {
			return errors.New("missing exception identifier after WITH")
		}
		if !isSPDXIDString(exc) || isSPDXReserved(exc) || strings.HasPrefix(exc, "LicenseRef-") || strings.HasPrefix(exc, "DocumentRef-") {
			return fmt.Errorf("invalid SPDX exception identifier %q", exc)
		}
		p.pos++
	}
	return nil
}

func (p *spdxParser) parseSimple() error {
	tok := p.peek()
	if tok == "" {
		return errors.New("unexpected end of SPDX expression")
	}
	if tok == ")" || isSPDXReserved(tok) {
		return fmt.Errorf("unexpected token %q in SPDX expression", tok)
	}
	if isSPDXLicenseRef(tok) {
		p.pos++
		return nil
	}
	id := strings.TrimSuffix(tok, "+")
	if !isSPDXIDString(id) || isSPDXReserved(id) || strings.HasPrefix(id, "LicenseRef-") || strings.HasPrefix(id, "DocumentRef-") {
		return fmt.Errorf("invalid SPDX license identifier %q", tok)
	}
	p.pos++
	return nil
}

func isSPDXReserved(tok string) bool {
	switch tok {
	case "AND", "OR", "WITH", "NONE", "NOASSERTION":
		return true
	}
	return false
}

func isSPDXLicenseRef(tok string) bool {
	if rest, ok := strings.CutPrefix(tok, "DocumentRef-"); ok {
		docID, licPart, hasColon := strings.Cut(rest, ":")
		if !hasColon || !isSPDXIDString(docID) {
			return false
		}
		tok = licPart
	}
	refID, ok := strings.CutPrefix(tok, "LicenseRef-")
	return ok && isSPDXIDString(refID)
}

func isSPDXIDString(s string) bool {
	if len(s) == 0 {
		return false
	}
	if !isASCIIAlphaNum(s[0]) || !isASCIIAlphaNum(s[len(s)-1]) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(isASCIIAlphaNum(c) || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

func isASCIIAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// sortedCopy returns a shallow copy of idx with Items sorted by Name.
func (idx *registryIndex) sortedCopy() *registryIndex {
	cp := &registryIndex{
		Version: idx.Version,
		Items:   make([]registryItem, len(idx.Items)),
	}
	copy(cp.Items, idx.Items)
	slices.SortFunc(cp.Items, func(a, b registryItem) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return cp
}

// encodeRegistryIndex encodes idx in canonical YAML form: sorted by name, fields in schema order, and two-space indent.
func encodeRegistryIndex(idx *registryIndex) ([]byte, error) {
	cp := idx.sortedCopy()
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(cp); err != nil {
		enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// checkRegistryPolicy verifies that raw is a valid version 1 registry index in canonical form and that no two names from different module owners are within one Levenshtein edit of each other.
func checkRegistryPolicy(raw []byte) error {
	idx, err := parseRegistryIndex(raw)
	if err != nil {
		return err
	}
	if !slices.IsSortedFunc(idx.Items, func(a, b registryItem) int { return cmp.Compare(a.Name, b.Name) }) {
		return errors.New("registry items must be sorted by name")
	}
	canonical, err := encodeRegistryIndex(idx)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, canonical) {
		return fmt.Errorf("registry file is not in canonical form; expected:\n%s", canonical)
	}
	for i := range idx.Items {
		for j := i + 1; j < len(idx.Items); j++ {
			a, b := idx.Items[i], idx.Items[j]
			if moduleOwner(a.Module) == moduleOwner(b.Module) {
				continue
			}
			if levenshteinDistance(a.Name, b.Name) <= 1 {
				return fmt.Errorf("names %q (%s) and %q (%s) are within one edit with different module owners (%s vs %s)",
					a.Name, a.Module, b.Name, b.Module, moduleOwner(a.Module), moduleOwner(b.Module))
			}
		}
	}
	return nil
}

// levenshteinDistance returns the Levenshtein edit distance between ASCII strings a and b.
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}
