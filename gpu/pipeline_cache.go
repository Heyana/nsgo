package gpu

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const pipelineCacheFormatVersion = 1

// PipelineCacheStats reports cache hit, miss, store, and compilation counts for a PipelineCache.
type PipelineCacheStats struct {
	Hits         uint64
	Misses       uint64
	Stores       uint64
	Entries      int
	Compilations uint64
}

type pipelineCacheRecord struct {
	Version   int                          `json:"version"`
	Key       string                       `json:"key"`
	Label     string                       `json:"label,omitempty"`
	Artifacts *driver.GPUPipelineArtifacts `json:"artifacts"`
}

// PipelineCache persists compiled GPU pipeline target artifacts (MSL, HLSL, GLSL 3.30, GLSL ES 3.00, WGSL, and optional backend PSO/binary archive blobs) across launches so subsequent runs reuse cached pipelines without shader recompilation or first-draw hitches.
//
// Metadata and portable shader artifacts are stored as deterministic, versioned JSON files (never encoding/gob). PipelineCache is safe for concurrent use by asynchronous precompilation goroutines.
type PipelineCache struct {
	mu           sync.RWMutex
	dir          string
	entries      map[string]*driver.GPUPipelineArtifacts
	blobs        map[string][]byte
	hits         uint64
	misses       uint64
	stores       uint64
	compilations uint64
}

// NewPipelineCache opens or creates a persistent PipelineCache backed by dir. When dir is empty, the cache operates in memory only. Existing valid cache entries in dir are loaded immediately so a second launch hits the cache with zero shader recompilation.
func NewPipelineCache(dir string) (*PipelineCache, error) {
	c := &PipelineCache{
		dir:     dir,
		entries: map[string]*driver.GPUPipelineArtifacts{},
		blobs:   map[string][]byte{},
	}
	if dir == "" {
		return c, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("gpu: create pipeline cache dir: %w", err)
	}
	if err := c.loadFromDir(); err != nil {
		return nil, err
	}
	return c, nil
}

// NewMemoryPipelineCache creates an in-memory PipelineCache with no disk I/O.
func NewMemoryPipelineCache() *PipelineCache {
	return &PipelineCache{
		entries: map[string]*driver.GPUPipelineArtifacts{},
		blobs:   map[string][]byte{},
	}
}

// Dir returns the cache's backing directory, or "" for an in-memory cache.
func (c *PipelineCache) Dir() string {
	if c == nil {
		return ""
	}
	return c.dir
}

// CacheStats returns a snapshot of the cache's hit, miss, store, entry, and shader compilation counters.
func (c *PipelineCache) CacheStats() PipelineCacheStats {
	if c == nil {
		return PipelineCacheStats{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return PipelineCacheStats{
		Hits:         c.hits,
		Misses:       c.misses,
		Stores:       c.stores,
		Entries:      len(c.entries),
		Compilations: c.compilations,
	}
}

// ResetStats zeroes hit, miss, store, and compilation counters while keeping all cached entries intact.
func (c *PipelineCache) ResetStats() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.hits = 0
	c.misses = 0
	c.stores = 0
	c.compilations = 0
	c.mu.Unlock()
}

// Has reports whether an entry with deterministic key exists in the cache.
func (c *PipelineCache) Has(key string) bool {
	if c == nil || key == "" {
		return false
	}
	c.mu.RLock()
	_, ok := c.entries[key]
	c.mu.RUnlock()
	return ok
}

// Clear removes all cached pipeline entries and backend blobs from memory and from the backing directory (if any).
func (c *PipelineCache) Clear() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.entries)
	clear(c.blobs)
	c.hits = 0
	c.misses = 0
	c.stores = 0
	c.compilations = 0
	if c.dir == "" {
		return nil
	}
	des, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".bin") {
			_ = os.Remove(filepath.Join(c.dir, name))
		}
	}
	return nil
}

// PutBackendBlob persists an opaque backend-native compiled pipeline cache blob (for example a Metal binary archive, Vulkan VkPipelineCache, or D3D11/12 PSO library) associated with key and backend name (e.g. "metal", "vulkan", "d3d11").
func (c *PipelineCache) PutBackendBlob(key, backend string, data []byte) error {
	if c == nil || key == "" || backend == "" {
		return nil
	}
	safeBackend := sanitizeCacheToken(backend)
	if safeBackend == "" {
		return Error{Code: ErrInvalidDescriptor, Message: "invalid backend cache token"}
	}
	cp := append([]byte(nil), data...)
	bk := key + "." + safeBackend
	c.mu.Lock()
	c.blobs[bk] = cp
	dir := c.dir
	c.mu.Unlock()
	if dir == "" {
		return nil
	}
	return writeFileAtomic(filepath.Join(dir, bk+".bin"), cp)
}

// GetBackendBlob retrieves a previously stored backend-native binary cache blob for key and backend name.
func (c *PipelineCache) GetBackendBlob(key, backend string) ([]byte, bool) {
	if c == nil || key == "" || backend == "" {
		return nil, false
	}
	safeBackend := sanitizeCacheToken(backend)
	if safeBackend == "" {
		return nil, false
	}
	bk := key + "." + safeBackend
	c.mu.RLock()
	b, ok := c.blobs[bk]
	dir := c.dir
	c.mu.RUnlock()
	if ok {
		return append([]byte(nil), b...), true
	}
	if dir == "" {
		return nil, false
	}
	raw, err := os.ReadFile(filepath.Join(dir, bk+".bin"))
	if err != nil {
		return nil, false
	}
	c.mu.Lock()
	c.blobs[bk] = raw
	c.mu.Unlock()
	return append([]byte(nil), raw...), true
}

func sanitizeCacheToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (c *PipelineCache) loadFromDir() error {
	des, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("gpu: read pipeline cache dir: %w", err)
	}
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(c.dir, name))
		if err != nil {
			continue
		}
		var rec pipelineCacheRecord
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&rec); err != nil {
			// Skip corrupt or incompatible entries gracefully.
			continue
		}
		if rec.Version != pipelineCacheFormatVersion || rec.Key == "" || rec.Artifacts == nil {
			continue
		}
		wantName := rec.Key + ".json"
		if name != wantName {
			continue
		}
		c.entries[rec.Key] = rec.Artifacts
	}
	return nil
}

func (c *PipelineCache) lookup(key string) (*driver.GPUPipelineArtifacts, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if art, ok := c.entries[key]; ok {
		c.hits++
		return art, true
	}
	c.misses++
	return nil, false
}

func (c *PipelineCache) store(key, label string, art *driver.GPUPipelineArtifacts) {
	if c == nil || key == "" || art == nil {
		return
	}
	c.mu.Lock()
	c.entries[key] = art
	c.stores++
	c.compilations++
	dir := c.dir
	c.mu.Unlock()
	if dir == "" {
		return
	}
	rec := pipelineCacheRecord{
		Version:   pipelineCacheFormatVersion,
		Key:       key,
		Label:     label,
		Artifacts: art,
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return
	}
	data = append(data, '\n')
	_ = writeFileAtomic(filepath.Join(dir, key+".json"), data)
}

func writeFileAtomic(dst string, data []byte) error {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".nsgo-gpu-cache-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// computePipelineCacheKey derives a deterministic SHA-256 hex digest from the shader source, entry points, bind group layout schemas, vertex layouts, primitive state, depth/stencil state, color targets, and sample count. Runtime handle IDs and pointer addresses are never hashed so keys stay identical across app launches.
func computePipelineCacheKey(desc PipelineDesc, dd driver.GPUPipelineDesc) string {
	h := sha256.New()
	var buf [8]byte
	writeU32 := func(v uint32) {
		binary.LittleEndian.PutUint32(buf[:4], v)
		_, _ = h.Write(buf[:4])
	}
	writeU64 := func(v uint64) {
		binary.LittleEndian.PutUint64(buf[:8], v)
		_, _ = h.Write(buf[:8])
	}
	writeU8 := func(v uint8) {
		buf[0] = v
		_, _ = h.Write(buf[:1])
	}
	writeBool := func(b bool) {
		if b {
			writeU8(1)
		} else {
			writeU8(0)
		}
	}
	writeStr := func(s string) {
		writeU32(uint32(len(s)))
		_, _ = h.Write([]byte(s))
	}

	writeU32(pipelineCacheFormatVersion)
	if desc.Vertex.Module != nil {
		writeStr(desc.Vertex.Module.source)
	} else {
		writeStr("")
	}
	writeStr(desc.Vertex.EntryPoint)
	if desc.Fragment.Module != nil {
		writeStr(desc.Fragment.Module.source)
	} else {
		writeStr("")
	}
	writeStr(desc.Fragment.EntryPoint)

	// Bind group layout schemas (not runtime GPUIDs).
	writeU32(uint32(len(desc.Layouts)))
	for _, l := range desc.Layouts {
		if l == nil {
			writeU32(0)
			continue
		}
		writeU32(uint32(len(l.entries)))
		for _, e := range l.entries {
			vis := e.Visibility
			if vis == 0 {
				vis = StageAll
			}
			writeU32(e.Binding)
			writeU8(uint8(vis))
			writeU8(uint8(e.Type))
			writeBool(e.HasDynamicOffset)
		}
	}

	// Vertex buffer layouts.
	writeU32(uint32(len(dd.VertexBuffers)))
	for _, vb := range dd.VertexBuffers {
		writeU64(vb.Stride)
		writeU8(uint8(vb.StepMode))
		writeU32(uint32(len(vb.Attributes)))
		for _, a := range vb.Attributes {
			writeU32(a.Location)
			writeU64(a.Offset)
			writeU8(uint8(a.Format))
		}
	}

	// Primitive state.
	writeU8(uint8(dd.Primitive.Topology))
	writeU8(uint8(dd.Primitive.FrontFace))
	writeU8(uint8(dd.Primitive.CullMode))

	// Depth/stencil state.
	writeBool(dd.DepthStencil != nil)
	if dd.DepthStencil != nil {
		writeU8(uint8(dd.DepthStencil.Format))
		writeBool(dd.DepthStencil.DepthWrite)
		writeU8(uint8(dd.DepthStencil.DepthCompare))
		writeU32(uint32(dd.DepthStencil.DepthBias))
		writeU32(math.Float32bits(dd.DepthStencil.DepthBiasSlopeScale))
		// Stencil state and the bias clamp are hashed only when set, so keys of pipelines without them stay unchanged.
		if desc.DepthStencil != nil && (desc.DepthStencil.usesStencilTest() || dd.DepthStencil.DepthBiasClamp != 0) {
			for _, f := range [2]driver.GPUStencilFaceState{dd.DepthStencil.StencilFront, dd.DepthStencil.StencilBack} {
				writeU8(uint8(f.Compare))
				writeU8(uint8(f.FailOp))
				writeU8(uint8(f.DepthFailOp))
				writeU8(uint8(f.PassOp))
			}
			writeU32(dd.DepthStencil.StencilReadMask)
			writeU32(dd.DepthStencil.StencilWriteMask)
			writeU32(math.Float32bits(dd.DepthStencil.DepthBiasClamp))
		}
	}

	// Color targets.
	writeU32(uint32(len(dd.Targets)))
	for _, t := range dd.Targets {
		writeU8(uint8(t.Format))
		writeU8(uint8(t.WriteMask))
		writeBool(t.Blend != nil)
		if t.Blend != nil {
			writeU8(uint8(t.Blend.Color.SrcFactor))
			writeU8(uint8(t.Blend.Color.DstFactor))
			writeU8(uint8(t.Blend.Color.Operation))
			writeU8(uint8(t.Blend.Alpha.SrcFactor))
			writeU8(uint8(t.Blend.Alpha.DstFactor))
			writeU8(uint8(t.Blend.Alpha.Operation))
		}
	}

	writeU32(uint32(dd.SampleCount))
	return hex.EncodeToString(h.Sum(nil))
}

func compileOrLoadPipelineArtifacts(cache *PipelineCache, key string, dd driver.GPUPipelineDesc) (*driver.GPUPipelineArtifacts, error) {
	if cache != nil {
		if art, ok := cache.lookup(key); ok {
			return art, nil
		}
	}
	art, err := compilePipelineArtifacts(dd)
	if err != nil {
		return nil, err
	}
	if cache != nil {
		cache.store(key, dd.Label, art)
	}
	return art, nil
}
