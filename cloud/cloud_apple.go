//go:build darwin

package cloud

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

var (
	appleOnce sync.Once
	appleLib  uintptr

	selAlloc       objc.SEL
	selInit        objc.SEL
	selRelease     objc.SEL
	selDrain       objc.SEL
	selCount       objc.SEL
	selObjectAtIdx objc.SEL

	// Foundation notification & metadata key constants.
	kvDidChangeNotification objc.ID
	kvChangeReasonKey       objc.ID
	kvChangedKeysKey        objc.ID
	mdScopeUbiquitousDocs   objc.ID
	mdItemFSNameKey         objc.ID
	mdItemURLKey            objc.ID
	mdDidUpdateNote         objc.ID
	mdDidFinishNote         objc.ID
	mdUpdateAddedKey        objc.ID
	mdUpdateChangedKey      objc.ID
	mdUpdateRemovedKey      objc.ID
)

func initApple() {
	appleOnce.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		appleLib = lib
		selAlloc = objc.RegisterName("alloc")
		selInit = objc.RegisterName("init")
		selRelease = objc.RegisterName("release")
		selDrain = objc.RegisterName("drain")
		selCount = objc.RegisterName("count")
		selObjectAtIdx = objc.RegisterName("objectAtIndex:")

		kvDidChangeNotification = foundationConst(lib, "NSUbiquitousKeyValueStoreDidChangeExternallyNotification")
		kvChangeReasonKey = foundationConst(lib, "NSUbiquitousKeyValueStoreChangeReasonKey")
		kvChangedKeysKey = foundationConst(lib, "NSUbiquitousKeyValueStoreChangedKeysKey")
		mdScopeUbiquitousDocs = foundationConst(lib, "NSMetadataQueryUbiquitousDocumentsScope")
		mdItemFSNameKey = foundationConst(lib, "NSMetadataItemFSNameKey")
		mdItemURLKey = foundationConst(lib, "NSMetadataItemURLKey")
		mdDidUpdateNote = foundationConst(lib, "NSMetadataQueryDidUpdateNotification")
		mdDidFinishNote = foundationConst(lib, "NSMetadataQueryDidFinishGatheringNotification")
		mdUpdateAddedKey = foundationConst(lib, "NSMetadataQueryUpdateAddedItemsKey")
		mdUpdateChangedKey = foundationConst(lib, "NSMetadataQueryUpdateChangedItemsKey")
		mdUpdateRemovedKey = foundationConst(lib, "NSMetadataQueryUpdateRemovedItemsKey")
	})
}

func foundationConst(lib uintptr, name string) objc.ID {
	addr, err := purego.Dlsym(lib, name)
	if err != nil || addr == 0 {
		return 0
	}
	return objc.ID(**(**uintptr)(unsafe.Pointer(&addr)))
}

func sel(name string) objc.SEL { return objc.RegisterName(name) }

func withAutoreleasePool(fn func()) {
	initApple()
	cls := objc.GetClass("NSAutoreleasePool")
	if cls == 0 {
		fn()
		return
	}
	pool := objc.ID(cls).Send(selAlloc).Send(selInit)
	defer pool.Send(selDrain)
	fn()
}

func newNSString(s string) objc.ID {
	cls := objc.ID(objc.GetClass("NSString"))
	if len(s) == 0 {
		return cls.Send(selAlloc).Send(selInit)
	}
	b := []byte(s)
	const nsUTF8StringEncoding = uint(4)
	ns := cls.Send(selAlloc).Send(sel("initWithBytes:length:encoding:"), unsafe.Pointer(&b[0]), uint(len(b)), nsUTF8StringEncoding)
	runtime.KeepAlive(b)
	return ns
}

func goString(ns objc.ID) string {
	if ns == 0 {
		return ""
	}
	p := objc.Send[*byte](ns, sel("UTF8String"))
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

func newNSData(b []byte) objc.ID {
	cls := objc.ID(objc.GetClass("NSData"))
	if len(b) == 0 {
		return cls.Send(selAlloc).Send(selInit)
	}
	d := cls.Send(selAlloc).Send(sel("initWithBytes:length:"), unsafe.Pointer(&b[0]), uint(len(b)))
	runtime.KeepAlive(b)
	return d
}

func goBytes(data objc.ID) []byte {
	if data == 0 {
		return nil
	}
	n := objc.Send[uint](data, sel("length"))
	if n == 0 {
		return []byte{}
	}
	p := objc.Send[*byte](data, sel("bytes"))
	if p == nil {
		return []byte{}
	}
	out := make([]byte, n)
	copy(out, unsafe.Slice(p, n))
	return out
}

func nsDateToTime(date objc.ID) time.Time {
	if date == 0 {
		return time.Time{}
	}
	secs := objc.Send[float64](date, sel("timeIntervalSince1970"))
	return time.UnixMicro(int64(secs * 1e6)).UTC()
}

func nsError(errID objc.ID) error {
	if errID == 0 {
		return nil
	}
	desc := goString(errID.Send(sel("localizedDescription")))
	if desc == "" {
		return errors.New("cloud: Foundation error")
	}
	return fmt.Errorf("cloud: %s", desc)
}

func appleCloudAvailable() bool {
	initApple()
	fmClass := objc.GetClass("NSFileManager")
	if fmClass == 0 {
		return false
	}
	fm := objc.ID(fmClass).Send(sel("defaultManager"))
	if fm == 0 {
		return false
	}
	return fm.Send(sel("ubiquityIdentityToken")) != 0
}

func kvDefaultStore() (objc.ID, error) {
	if !appleCloudAvailable() {
		return 0, ErrUnavailable
	}
	cls := objc.GetClass("NSUbiquitousKeyValueStore")
	if cls == 0 {
		return 0, ErrUnavailable
	}
	store := objc.ID(cls).Send(sel("defaultStore"))
	if store == 0 {
		return 0, ErrUnavailable
	}
	return store, nil
}

func platformGet(key string) (val []byte, err error) {
	withAutoreleasePool(func() {
		store, e := kvDefaultStore()
		if e != nil {
			err = e
			return
		}
		k := newNSString(key)
		defer k.Send(selRelease)
		if data := store.Send(sel("dataForKey:"), k); data != 0 {
			val = goBytes(data)
			return
		}
		if str := store.Send(sel("stringForKey:"), k); str != 0 {
			val = []byte(goString(str))
			return
		}
		obj := store.Send(sel("objectForKey:"), k)
		if obj == 0 {
			err = ErrNotFound
			return
		}
		val = []byte(goString(obj.Send(sel("description"))))
	})
	return val, err
}

func platformSet(key string, value []byte) (err error) {
	withAutoreleasePool(func() {
		store, e := kvDefaultStore()
		if e != nil {
			err = e
			return
		}
		k := newNSString(key)
		defer k.Send(selRelease)
		d := newNSData(value)
		defer d.Send(selRelease)
		store.Send(sel("setObject:forKey:"), d, k)
		if !objc.Send[bool](store, sel("synchronize")) {
			err = ErrUnavailable
		}
	})
	return err
}

func platformDelete(key string) (err error) {
	withAutoreleasePool(func() {
		store, e := kvDefaultStore()
		if e != nil {
			err = e
			return
		}
		k := newNSString(key)
		defer k.Send(selRelease)
		store.Send(sel("removeObjectForKey:"), k)
		if !objc.Send[bool](store, sel("synchronize")) {
			err = ErrUnavailable
		}
	})
	return err
}

func platformKeys() (keys []string, err error) {
	withAutoreleasePool(func() {
		store, e := kvDefaultStore()
		if e != nil {
			err = e
			return
		}
		dict := store.Send(sel("dictionaryRepresentation"))
		if dict == 0 {
			keys = []string{}
			return
		}
		allKeys := dict.Send(sel("allKeys"))
		n := objc.Send[uint](allKeys, selCount)
		keys = make([]string, 0, n)
		for i := range n {
			item := allKeys.Send(selObjectAtIdx, i)
			keys = append(keys, goString(item))
		}
		slices.Sort(keys)
	})
	return keys, err
}

func ubiquityDocumentsDir() (string, error) {
	if !appleCloudAvailable() {
		return "", ErrUnavailable
	}
	fm := objc.ID(objc.GetClass("NSFileManager")).Send(sel("defaultManager"))
	if fm == 0 {
		return "", ErrUnavailable
	}
	rootURL := fm.Send(sel("URLForUbiquityContainerIdentifier:"), objc.ID(0))
	if rootURL == 0 {
		return "", ErrUnavailable
	}
	docsName := newNSString("Documents")
	defer docsName.Send(selRelease)
	docsURL := rootURL.Send(sel("URLByAppendingPathComponent:isDirectory:"), docsName, true)
	if docsURL == 0 {
		return "", ErrUnavailable
	}
	docsPath := goString(docsURL.Send(sel("path")))
	if docsPath == "" {
		return "", ErrUnavailable
	}
	if err := os.MkdirAll(docsPath, 0o755); err != nil {
		return "", err
	}
	return docsPath, nil
}

func fileURLForPath(p string) objc.ID {
	ns := newNSString(p)
	defer ns.Send(selRelease)
	return objc.ID(objc.GetClass("NSURL")).Send(sel("fileURLWithPath:"), ns)
}

// checkAppleFileConflict inspects NSFileVersion.unresolvedConflictVersionsOfItemAtURL: for fullPath. If unresolved conflicts exist, it notifies registered OnConflict handlers and returns a *ConflictError.
func checkAppleFileConflict(name, fullPath string) *ConflictError {
	verCls := objc.GetClass("NSFileVersion")
	if verCls == 0 {
		return nil
	}
	url := fileURLForPath(fullPath)
	if url == 0 {
		return nil
	}
	conflicts := objc.ID(verCls).Send(sel("unresolvedConflictVersionsOfItemAtURL:"), url)
	if conflicts == 0 {
		return nil
	}
	n := objc.Send[uint](conflicts, selCount)
	if n == 0 {
		return nil
	}
	localData, _ := os.ReadFile(fullPath)
	var localTime time.Time
	if info, err := os.Stat(fullPath); err == nil {
		localTime = info.ModTime().UTC()
	}
	first := conflicts.Send(selObjectAtIdx, uint(0))
	remoteURL := first.Send(sel("URL"))
	remotePath := goString(remoteURL.Send(sel("path")))
	remoteData, _ := os.ReadFile(remotePath)
	remoteTime := nsDateToTime(first.Send(sel("modificationDate")))

	conflict := Conflict{
		Name:       name,
		Local:      localData,
		Remote:     remoteData,
		LocalTime:  localTime,
		RemoteTime: remoteTime,
		Resolve: func(chosen []byte) error {
			return resolveAppleFileConflict(fullPath, chosen)
		},
	}
	dispatchAppleConflict(conflict)
	return &ConflictError{
		Key:        name,
		Local:      localData,
		Remote:     remoteData,
		LocalTime:  localTime,
		RemoteTime: remoteTime,
	}
}

func resolveAppleFileConflict(fullPath string, chosen []byte) (err error) {
	withAutoreleasePool(func() {
		if !appleCloudAvailable() {
			err = ErrUnavailable
			return
		}
		if werr := writeAtomicFile(fullPath, chosen); werr != nil {
			err = werr
			return
		}
		verCls := objc.GetClass("NSFileVersion")
		if verCls == 0 {
			return
		}
		url := fileURLForPath(fullPath)
		conflicts := objc.ID(verCls).Send(sel("unresolvedConflictVersionsOfItemAtURL:"), url)
		if conflicts != 0 {
			n := objc.Send[uint](conflicts, selCount)
			for i := range n {
				v := conflicts.Send(selObjectAtIdx, i)
				v.Send(sel("setResolved:"), true)
			}
		}
		var nsErr objc.ID
		ok := objc.Send[bool](objc.ID(verCls), sel("removeOtherVersionsOfItemAtURL:error:"), url, unsafe.Pointer(&nsErr))
		if !ok && nsErr != 0 {
			err = nsError(nsErr)
		}
	})
	return err
}

func writeAtomicFile(fullPath string, data []byte) error {
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".nsgo-cloud-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, fullPath); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

func platformReadFile(name string) (data []byte, err error) {
	withAutoreleasePool(func() {
		docsPath, e := ubiquityDocumentsDir()
		if e != nil {
			err = e
			return
		}
		full := filepath.Join(docsPath, filepath.FromSlash(name))
		if ce := checkAppleFileConflict(name, full); ce != nil {
			err = ce
			return
		}
		b, rerr := os.ReadFile(full)
		if errors.Is(rerr, os.ErrNotExist) {
			// Request download if the file is an evicted iCloud placeholder.
			fm := objc.ID(objc.GetClass("NSFileManager")).Send(sel("defaultManager"))
			if url := fileURLForPath(full); fm != 0 && url != 0 {
				var nsErr objc.ID
				fm.Send(sel("startDownloadingUbiquitousItemAtURL:error:"), url, unsafe.Pointer(&nsErr))
			}
			err = ErrNotFound
			return
		}
		data, err = b, rerr
	})
	return data, err
}

func platformWriteFile(name string, data []byte) (err error) {
	withAutoreleasePool(func() {
		docsPath, e := ubiquityDocumentsDir()
		if e != nil {
			err = e
			return
		}
		full := filepath.Join(docsPath, filepath.FromSlash(name))
		err = writeAtomicFile(full, data)
	})
	return err
}

func platformDeleteFile(name string) (err error) {
	withAutoreleasePool(func() {
		docsPath, e := ubiquityDocumentsDir()
		if e != nil {
			err = e
			return
		}
		full := filepath.Join(docsPath, filepath.FromSlash(name))
		rerr := os.Remove(full)
		if rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			err = rerr
		}
	})
	return err
}

func platformListFiles() (names []string, err error) {
	withAutoreleasePool(func() {
		docsPath, e := ubiquityDocumentsDir()
		if e != nil {
			err = e
			return
		}
		var out []string
		werr := filepath.WalkDir(docsPath, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			base := d.Name()
			if strings.HasPrefix(base, ".") {
				return nil
			}
			rel, rerr := filepath.Rel(docsPath, p)
			if rerr != nil {
				return rerr
			}
			out = append(out, filepath.ToSlash(rel))
			return nil
		})
		if werr != nil {
			err = werr
			return
		}
		slices.Sort(out)
		names = out
	})
	return names, err
}

// Apple change & conflict observer registry.
var appleWatch struct {
	mu          sync.Mutex
	nextID      uint64
	watchers    map[uint64]func(ChangeEvent)
	conflicts   map[uint64]func(Conflict)
	kvObserver  objc.ID
	kvBlock     objc.Block
	mdQuery     objc.ID
	mdObservers []objc.ID
	mdBlocks    []objc.Block
}

func platformWatchChanges(onChange func(ChangeEvent)) func() {
	appleWatch.mu.Lock()
	if appleWatch.watchers == nil {
		appleWatch.watchers = map[uint64]func(ChangeEvent){}
	}
	appleWatch.nextID++
	id := appleWatch.nextID
	appleWatch.watchers[id] = onChange
	needStart := len(appleWatch.watchers) == 1
	appleWatch.mu.Unlock()

	if needStart {
		startAppleObservers()
	}
	return func() {
		appleWatch.mu.Lock()
		delete(appleWatch.watchers, id)
		needStop := len(appleWatch.watchers) == 0
		appleWatch.mu.Unlock()
		if needStop {
			stopAppleObservers()
		}
	}
}

func platformOnConflict(resolve func(Conflict)) func() {
	appleWatch.mu.Lock()
	if appleWatch.conflicts == nil {
		appleWatch.conflicts = map[uint64]func(Conflict){}
	}
	appleWatch.nextID++
	id := appleWatch.nextID
	appleWatch.conflicts[id] = resolve
	appleWatch.mu.Unlock()

	return func() {
		appleWatch.mu.Lock()
		delete(appleWatch.conflicts, id)
		appleWatch.mu.Unlock()
	}
}

func dispatchAppleChange(ev ChangeEvent) {
	appleWatch.mu.Lock()
	list := make([]func(ChangeEvent), 0, len(appleWatch.watchers))
	for _, fn := range appleWatch.watchers {
		list = append(list, fn)
	}
	appleWatch.mu.Unlock()
	for _, fn := range list {
		fn(ev)
	}
}

func dispatchAppleConflict(c Conflict) {
	appleWatch.mu.Lock()
	list := make([]func(Conflict), 0, len(appleWatch.conflicts))
	for _, fn := range appleWatch.conflicts {
		list = append(list, fn)
	}
	appleWatch.mu.Unlock()
	for _, fn := range list {
		fn(c)
	}
}

func startAppleObservers() {
	withAutoreleasePool(func() {
		if !appleCloudAvailable() {
			return
		}
		centerCls := objc.GetClass("NSNotificationCenter")
		if centerCls == 0 {
			return
		}
		center := objc.ID(centerCls).Send(sel("defaultCenter"))
		if center == 0 {
			return
		}

		// 1. NSUbiquitousKeyValueStore external changes.
		store, err := kvDefaultStore()
		if err == nil && kvDidChangeNotification != 0 {
			block := objcblock.New(func(_ objc.Block, note objc.ID) {
				handleAppleKVChange(note)
			})
			obs := center.Send(sel("addObserverForName:object:queue:usingBlock:"), kvDidChangeNotification, store, objc.ID(0), block)
			appleWatch.mu.Lock()
			appleWatch.kvObserver = obs
			appleWatch.kvBlock = block
			appleWatch.mu.Unlock()
			objc.Send[bool](store, sel("synchronize"))
		}

		// 2. NSMetadataQuery for ubiquity document file changes & conflicts.
		queryCls := objc.GetClass("NSMetadataQuery")
		predCls := objc.GetClass("NSPredicate")
		arrCls := objc.GetClass("NSArray")
		if queryCls == 0 || predCls == 0 || arrCls == 0 || mdScopeUbiquitousDocs == 0 || mdItemFSNameKey == 0 {
			return
		}
		query := objc.ID(queryCls).Send(selAlloc).Send(selInit)
		if query == 0 {
			return
		}
		scopes := objc.ID(arrCls).Send(sel("arrayWithObject:"), mdScopeUbiquitousDocs)
		query.Send(sel("setSearchScopes:"), scopes)
		fmtStr := newNSString("%K LIKE '*'")
		pred := objc.ID(predCls).Send(sel("predicateWithFormat:"), fmtStr, mdItemFSNameKey)
		fmtStr.Send(selRelease)
		query.Send(sel("setPredicate:"), pred)

		var observers []objc.ID
		var blocks []objc.Block
		for _, pair := range []struct {
			note   objc.ID
			reason ChangeReason
		}{
			{mdDidFinishNote, ChangeInitialSync},
			{mdDidUpdateNote, ChangeServer},
		} {
			if pair.note == 0 {
				continue
			}
			reason := pair.reason
			b := objcblock.New(func(_ objc.Block, note objc.ID) {
				handleAppleMetadataQuery(query, note, reason)
			})
			obs := center.Send(sel("addObserverForName:object:queue:usingBlock:"), pair.note, query, objc.ID(0), b)
			observers = append(observers, obs)
			blocks = append(blocks, b)
		}

		appleWatch.mu.Lock()
		appleWatch.mdQuery = query
		appleWatch.mdObservers = observers
		appleWatch.mdBlocks = blocks
		appleWatch.mu.Unlock()

		// NSMetadataQuery must start on a thread with an active run loop (the main queue).
		if opQueueCls := objc.GetClass("NSOperationQueue"); opQueueCls != 0 {
			mainQ := objc.ID(opQueueCls).Send(sel("mainQueue"))
			if mainQ != 0 {
				startBlock := objcblock.New(func(_ objc.Block) {
					query.Send(sel("startQuery"))
				})
				mainQ.Send(sel("addOperationWithBlock:"), startBlock)
				startBlock.Release()
			}
		}
	})
}

func stopAppleObservers() {
	withAutoreleasePool(func() {
		appleWatch.mu.Lock()
		kvObs, kvBlock := appleWatch.kvObserver, appleWatch.kvBlock
		query, mdObs, mdBlocks := appleWatch.mdQuery, appleWatch.mdObservers, appleWatch.mdBlocks
		appleWatch.kvObserver, appleWatch.kvBlock = 0, 0
		appleWatch.mdQuery, appleWatch.mdObservers, appleWatch.mdBlocks = 0, nil, nil
		appleWatch.mu.Unlock()

		centerCls := objc.GetClass("NSNotificationCenter")
		var center objc.ID
		if centerCls != 0 {
			center = objc.ID(centerCls).Send(sel("defaultCenter"))
		}
		if kvObs != 0 && center != 0 {
			center.Send(sel("removeObserver:"), kvObs)
		}
		if kvBlock != 0 {
			kvBlock.Release()
		}
		for _, obs := range mdObs {
			if obs != 0 && center != 0 {
				center.Send(sel("removeObserver:"), obs)
			}
		}
		for _, b := range mdBlocks {
			if b != 0 {
				b.Release()
			}
		}
		if query != 0 {
			query.Send(sel("stopQuery"))
			query.Send(selRelease)
		}
	})
}

func handleAppleKVChange(note objc.ID) {
	if note == 0 {
		return
	}
	userInfo := note.Send(sel("userInfo"))
	if userInfo == 0 {
		return
	}
	reason := ChangeServer
	if kvChangeReasonKey != 0 {
		if num := userInfo.Send(sel("objectForKey:"), kvChangeReasonKey); num != 0 {
			switch objc.Send[int](num, sel("integerValue")) {
			case 0:
				reason = ChangeServer
			case 1:
				reason = ChangeInitialSync
			case 2:
				reason = ChangeQuotaViolation
			case 3:
				reason = ChangeAccount
			}
		}
	}
	var keys []string
	if kvChangedKeysKey != 0 {
		if arr := userInfo.Send(sel("objectForKey:"), kvChangedKeysKey); arr != 0 {
			n := objc.Send[uint](arr, selCount)
			keys = make([]string, 0, n)
			for i := range n {
				keys = append(keys, goString(arr.Send(selObjectAtIdx, i)))
			}
			slices.Sort(keys)
		}
	}
	dispatchAppleChange(ChangeEvent{Keys: keys, Reason: reason})
}

func handleAppleMetadataQuery(query, note objc.ID, reason ChangeReason) {
	if query == 0 {
		return
	}
	query.Send(sel("disableUpdates"))
	defer query.Send(sel("enableUpdates"))

	docsPath, err := ubiquityDocumentsDir()
	if err != nil {
		return
	}
	seen := map[string]bool{}
	var files []string
	collect := func(arr objc.ID) {
		if arr == 0 {
			return
		}
		n := objc.Send[uint](arr, selCount)
		for i := range n {
			item := arr.Send(selObjectAtIdx, i)
			if item == 0 || mdItemURLKey == 0 {
				continue
			}
			url := item.Send(sel("valueForAttribute:"), mdItemURLKey)
			if url == 0 {
				continue
			}
			p := goString(url.Send(sel("path")))
			if p == "" {
				continue
			}
			rel, rerr := filepath.Rel(docsPath, p)
			if rerr != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			rel = filepath.ToSlash(rel)
			if !seen[rel] {
				seen[rel] = true
				files = append(files, rel)
				// Surface any unresolved iCloud file version conflicts immediately.
				checkAppleFileConflict(rel, p)
			}
		}
	}
	if reason == ChangeInitialSync {
		collect(query.Send(sel("results")))
	} else if note != 0 {
		if info := note.Send(sel("userInfo")); info != 0 {
			for _, k := range []objc.ID{mdUpdateAddedKey, mdUpdateChangedKey, mdUpdateRemovedKey} {
				if k != 0 {
					collect(info.Send(sel("objectForKey:"), k))
				}
			}
		}
	}
	if len(files) == 0 {
		return
	}
	slices.Sort(files)
	dispatchAppleChange(ChangeEvent{Files: files, Reason: reason})
}
