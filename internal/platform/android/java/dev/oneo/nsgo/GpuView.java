package dev.oneo.nsgo;

import android.app.ActivityManager;
import android.content.Context;
import android.content.pm.ConfigurationInfo;
import android.graphics.SurfaceTexture;
import android.opengl.EGL14;
import android.opengl.EGLConfig;
import android.opengl.EGLContext;
import android.opengl.EGLDisplay;
import android.opengl.EGLSurface;
import android.opengl.GLES30;
import android.os.Handler;
import android.util.Log;
import android.view.TextureView;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * A 3D GPU view (driver.KindGPU, docs/architecture/gpu.md): a TextureView backed
 * by OpenGL ES 3.0 on the runtime's shared GL HandlerThread (ShaderNode). Go
 * compiles shaderir programs to GLSL ES 3.00 and streams compact binary GPU
 * frames off-commit over Protocol.EXT_GPU_FRAME; this view manages retained GL
 * buffers, textures, samplers, bind groups, pipelines, VAOs, and offscreen FBOs,
 * and reports lifecycle, resize, error, and readback events over Protocol.GPU_EVENT.
 */
final class GpuView extends TextureView implements TextureView.SurfaceTextureListener {
    private static final String TAG = "nsgo";

    // GL_EXT_texture_filter_anisotropic constants.
    private static final int GL_TEXTURE_MAX_ANISOTROPY_EXT = 0x84FE;
    private static final int GL_MAX_TEXTURE_MAX_ANISOTROPY_EXT = 0x84FF;

    // driver.GPUTextureFormat
    static final int FORMAT_UNDEFINED = 0;
    static final int FORMAT_RGBA8_UNORM = 1;
    static final int FORMAT_RGBA8_UNORM_SRGB = 2;
    static final int FORMAT_BGRA8_UNORM = 3;
    static final int FORMAT_RG8_UNORM = 4;
    static final int FORMAT_R8_UNORM = 5;
    static final int FORMAT_RGBA16_FLOAT = 6;
    static final int FORMAT_DEPTH24_PLUS = 7;
    static final int FORMAT_DEPTH24_PLUS_STENCIL8 = 8;
    static final int FORMAT_DEPTH32_FLOAT = 9;

    // driver.GPUTextureDimension
    static final int DIM_2D = 0;
    static final int DIM_CUBE = 1;
    static final int DIM_2D_ARRAY = 2;

    // driver.GPUBufferUsage
    static final int BUF_VERTEX = 1 << 0;
    static final int BUF_INDEX = 1 << 1;
    static final int BUF_UNIFORM = 1 << 2;
    static final int BUF_COPY_DST = 1 << 5;

    // driver.GPUFilterMode
    static final int FILTER_NEAREST = 0;
    static final int FILTER_LINEAR = 1;

    // driver.GPUAddressMode
    static final int ADDR_CLAMP_TO_EDGE = 0;
    static final int ADDR_REPEAT = 1;
    static final int ADDR_MIRROR_REPEAT = 2;

    // driver.GPUCompareFunc
    static final int COMPARE_UNDEFINED = 0;
    static final int COMPARE_NEVER = 1;
    static final int COMPARE_LESS = 2;
    static final int COMPARE_EQUAL = 3;
    static final int COMPARE_LESS_EQUAL = 4;
    static final int COMPARE_GREATER = 5;
    static final int COMPARE_NOT_EQUAL = 6;
    static final int COMPARE_GREATER_EQUAL = 7;
    static final int COMPARE_ALWAYS = 8;

    // driver.GPUVertexFormat
    static final int VTX_FLOAT32 = 1;
    static final int VTX_FLOAT32X2 = 2;
    static final int VTX_FLOAT32X3 = 3;
    static final int VTX_FLOAT32X4 = 4;
    static final int VTX_SINT32 = 5;
    static final int VTX_SINT32X2 = 6;
    static final int VTX_SINT32X3 = 7;
    static final int VTX_SINT32X4 = 8;
    static final int VTX_UINT32 = 9;
    static final int VTX_UINT32X2 = 10;
    static final int VTX_UINT32X3 = 11;
    static final int VTX_UINT32X4 = 12;
    static final int VTX_UNORM8X4 = 13;

    // driver.GPUVertexStepMode
    static final int STEP_VERTEX = 0;
    static final int STEP_INSTANCE = 1;

    // driver.GPUIndexFormat
    static final int INDEX_UINT16 = 0;
    static final int INDEX_UINT32 = 1;

    // driver.GPUPrimitiveTopology
    static final int TOPO_TRIANGLE_LIST = 0;
    static final int TOPO_TRIANGLE_STRIP = 1;
    static final int TOPO_LINE_LIST = 2;
    static final int TOPO_LINE_STRIP = 3;
    static final int TOPO_POINT_LIST = 4;

    // driver.GPUFrontFace
    static final int FRONT_CCW = 0;
    static final int FRONT_CW = 1;

    // driver.GPUCullMode
    static final int CULL_NONE = 0;
    static final int CULL_FRONT = 1;
    static final int CULL_BACK = 2;

    // driver.GPUBlendFactor
    static final int BLEND_ZERO = 0;
    static final int BLEND_ONE = 1;
    static final int BLEND_SRC = 2;
    static final int BLEND_ONE_MINUS_SRC = 3;
    static final int BLEND_SRC_ALPHA = 4;
    static final int BLEND_ONE_MINUS_SRC_ALPHA = 5;
    static final int BLEND_DST = 6;
    static final int BLEND_ONE_MINUS_DST = 7;
    static final int BLEND_DST_ALPHA = 8;
    static final int BLEND_ONE_MINUS_DST_ALPHA = 9;
    static final int BLEND_CONSTANT = 10;
    static final int BLEND_ONE_MINUS_CONSTANT = 11;

    // driver.GPUBlendOperation
    static final int BLEND_ADD = 0;
    static final int BLEND_SUBTRACT = 1;
    static final int BLEND_REVERSE_SUBTRACT = 2;
    static final int BLEND_MIN = 3;
    static final int BLEND_MAX = 4;

    // driver.GPULoadOp
    static final int LOAD_CLEAR = 0;

    // driver.GPUResourceOpKind
    static final int OP_CREATE_BUFFER = 1;
    static final int OP_WRITE_BUFFER = 2;
    static final int OP_DESTROY_BUFFER = 3;
    static final int OP_CREATE_TEXTURE = 4;
    static final int OP_WRITE_TEXTURE = 5;
    static final int OP_DESTROY_TEXTURE = 6;
    static final int OP_CREATE_SAMPLER = 7;
    static final int OP_DESTROY_SAMPLER = 8;
    static final int OP_CREATE_BIND_GROUP_LAYOUT = 9;
    static final int OP_DESTROY_BIND_GROUP_LAYOUT = 10;
    static final int OP_CREATE_BIND_GROUP = 11;
    static final int OP_DESTROY_BIND_GROUP = 12;
    static final int OP_CREATE_PIPELINE = 13;
    static final int OP_DESTROY_PIPELINE = 14;
    static final int OP_READ_TEXTURE = 15;

    // driver.GPUCopyOpKind
    static final int COPY_BUFFER_TO_BUFFER = 1;
    static final int COPY_BUFFER_TO_TEXTURE = 2;
    static final int COPY_TEXTURE_TO_TEXTURE = 3;

    // driver.GPUCommandKind
    static final int CMD_SET_VIEWPORT = 1;
    static final int CMD_SET_SCISSOR = 2;
    static final int CMD_SET_PIPELINE = 3;
    static final int CMD_SET_BIND_GROUP = 4;
    static final int CMD_SET_VERTEX_BUFFER = 5;
    static final int CMD_SET_INDEX_BUFFER = 6;
    static final int CMD_SET_STENCIL_REFERENCE = 7;
    static final int CMD_SET_BLEND_CONSTANT = 8;
    static final int CMD_DRAW = 9;
    static final int CMD_DRAW_INDEXED = 10;
    static final int CMD_DRAW_INSTANCED = 11;
    static final int CMD_DRAW_INDEXED_INSTANCED = 12;

    // driver.GPUEventKind
    static final int EVENT_DEVICE_READY = 1;
    static final int EVENT_DEVICE_LOST = 2;
    static final int EVENT_RESIZE = 3;
    static final int EVENT_ERROR = 4;
    static final int EVENT_READBACK = 5;

    // driver.GPUDeviceLostReason
    static final int LOST_UNKNOWN = 0;
    static final int LOST_RESET = 1;
    static final int LOST_DESTROYED = 2;
    static final int LOST_OUT_OF_MEMORY = 3;

    // driver.GPUErrorCode
    static final int ERR_UNSUPPORTED = 1;
    static final int ERR_INVALID_DESCRIPTOR = 2;
    static final int ERR_SHADER_COMPILE = 3;
    static final int ERR_OUT_OF_MEMORY = 4;
    static final int ERR_DEVICE_LOST = 5;
    static final int ERR_FAILED = 6;

    interface Reporter {
        void send(byte[] payload);
    }

    /** Active GpuViews indexed by (surfaceId, viewId) so EXT_GPU_FRAME bypasses the UI thread. */
    private static final ConcurrentHashMap<Long, GpuView> activeViews = new ConcurrentHashMap<>();

    private static long viewKey(int surfaceId, int viewId) {
        return (((long) surfaceId) << 32) | (viewId & 0xffffffffL);
    }

    /** Reports whether this Android device advertises OpenGL ES 3.0+. */
    static boolean available(Context context) {
        if (context == null) return false;
        ActivityManager am = context.getSystemService(ActivityManager.class);
        if (am == null) return false;
        ConfigurationInfo info = am.getDeviceConfigurationInfo();
        return info != null && info.reqGlEsVersion >= 0x30000;
    }

    /** Routes a binary Protocol.EXT_GPU_FRAME directly to its target view's GL thread queue. */
    static boolean dispatch(int surfaceId, int viewId, byte[] payload) {
        GpuView view = activeViews.get(viewKey(surfaceId, viewId));
        if (view == null) return false;
        view.enqueueFrame(payload);
        return true;
    }

    // Retained GPU objects, accessed exclusively on the shared GL thread.
    private static final class BufferObj {
        final int glId;
        final int target;
        final long size;
        final int usage;

        BufferObj(int glId, int target, long size, int usage) {
            this.glId = glId;
            this.target = target;
            this.size = size;
            this.usage = usage;
        }
    }

    private static final class TextureObj {
        final int glId;
        final int target;
        final int width, height, layers, mipLevels, sampleCount;
        final int format;
        final int internalFormat, pixelFormat, pixelType, bpp;
        final boolean isDepth;
        final boolean isRenderbuffer;

        TextureObj(int glId, int target, int width, int height, int layers, int mipLevels, int sampleCount,
                int format, int internalFormat, int pixelFormat, int pixelType, int bpp,
                boolean isDepth, boolean isRenderbuffer) {
            this.glId = glId;
            this.target = target;
            this.width = width;
            this.height = height;
            this.layers = layers;
            this.mipLevels = mipLevels;
            this.sampleCount = sampleCount;
            this.format = format;
            this.internalFormat = internalFormat;
            this.pixelFormat = pixelFormat;
            this.pixelType = pixelType;
            this.bpp = bpp;
            this.isDepth = isDepth;
            this.isRenderbuffer = isRenderbuffer;
        }
    }

    private static final class BindGroupEntryObj {
        final int binding;
        final long bufferId;
        final long offset;
        final long size;
        final long textureId;
        final long samplerId;

        BindGroupEntryObj(int binding, long bufferId, long offset, long size, long textureId, long samplerId) {
            this.binding = binding;
            this.bufferId = bufferId;
            this.offset = offset;
            this.size = size;
            this.textureId = textureId;
            this.samplerId = samplerId;
        }
    }

    private static final class BindGroupObj {
        final long layoutId;
        final Map<Integer, BindGroupEntryObj> entries = new HashMap<>();

        BindGroupObj(long layoutId) {
            this.layoutId = layoutId;
        }
    }

    private static final class UniformBlockSlot {
        final int group, binding, slot;
        final String blockName;

        UniformBlockSlot(int group, int binding, String blockName, int slot) {
            this.group = group;
            this.binding = binding;
            this.blockName = blockName;
            this.slot = slot;
        }
    }

    private static final class SamplerPair {
        final String name;
        final int unit;
        final int texGroup, texBinding;
        final int sampGroup, sampBinding;

        SamplerPair(String name, int unit, int texGroup, int texBinding, int sampGroup, int sampBinding) {
            this.name = name;
            this.unit = unit;
            this.texGroup = texGroup;
            this.texBinding = texBinding;
            this.sampGroup = sampGroup;
            this.sampBinding = sampBinding;
        }
    }

    private static final class VertexAttr {
        final int format;
        final long offset;
        final int location;

        VertexAttr(int format, long offset, int location) {
            this.format = format;
            this.offset = offset;
            this.location = location;
        }
    }

    private static final class VertexBufferLayout {
        final long stride;
        final int stepMode;
        final VertexAttr[] attrs;

        VertexBufferLayout(long stride, int stepMode, VertexAttr[] attrs) {
            this.stride = stride;
            this.stepMode = stepMode;
            this.attrs = attrs;
        }
    }

    private static final class PipelineObj {
        final int program;
        final int vao;
        final UniformBlockSlot[] uniformBlocks;
        final SamplerPair[] samplerPairs;
        final VertexBufferLayout[] vertexBuffers;
        final int glTopology;
        final int glFrontFace;
        final int cullMode;
        final boolean depthEnabled;
        final boolean depthWrite;
        final int glDepthCompare;
        final int depthBias;
        final float depthBiasSlopeScale;
        final int writeMask;
        final boolean blendEnabled;
        final int srcColor, dstColor, opColor;
        final int srcAlpha, dstAlpha, opAlpha;

        PipelineObj(int program, int vao, UniformBlockSlot[] uniformBlocks, SamplerPair[] samplerPairs,
                VertexBufferLayout[] vertexBuffers, int glTopology, int glFrontFace, int cullMode,
                boolean depthEnabled, boolean depthWrite, int glDepthCompare, int depthBias, float depthBiasSlopeScale,
                int writeMask, boolean blendEnabled, int srcColor, int dstColor, int opColor,
                int srcAlpha, int dstAlpha, int opAlpha) {
            this.program = program;
            this.vao = vao;
            this.uniformBlocks = uniformBlocks;
            this.samplerPairs = samplerPairs;
            this.vertexBuffers = vertexBuffers;
            this.glTopology = glTopology;
            this.glFrontFace = glFrontFace;
            this.cullMode = cullMode;
            this.depthEnabled = depthEnabled;
            this.depthWrite = depthWrite;
            this.glDepthCompare = glDepthCompare;
            this.depthBias = depthBias;
            this.depthBiasSlopeScale = depthBiasSlopeScale;
            this.writeMask = writeMask;
            this.blendEnabled = blendEnabled;
            this.srcColor = srcColor;
            this.dstColor = dstColor;
            this.opColor = opColor;
            this.srcAlpha = srcAlpha;
            this.dstAlpha = dstAlpha;
            this.opAlpha = opAlpha;
        }
    }

    private final int surfaceId;
    private final int viewId;
    private final Reporter reporter;
    private volatile boolean destroyed;

    // View configuration from ItemGPU.
    private volatile int requestedColorFormat = FORMAT_RGBA8_UNORM;
    private volatile int requestedDepthFormat = FORMAT_UNDEFINED;
    private volatile int requestedSampleCount = 1;

    // GL-thread-only state.
    private SurfaceTexture activeSurfaceTexture;
    private EGLSurface eglSurface;
    private int width, height;
    private int generation;
    private boolean ready;
    private boolean anisotropySupported;
    private float maxAnisotropy = 1f;
    private int maxSamples = 1;
    private int offscreenFbo;
    private int readFbo;
    private int drawFbo;
    private int maxAttachedColors;

    private final Map<Long, BufferObj> buffers = new HashMap<>();
    private final Map<Long, TextureObj> textures = new HashMap<>();
    private final Map<Long, Integer> samplers = new HashMap<>();
    private final Map<Long, Map<Integer, Integer>> bindGroupLayoutDynSlots = new HashMap<>();
    private final Map<Long, BindGroupObj> bindGroups = new HashMap<>();
    private final Map<Long, PipelineObj> pipelines = new HashMap<>();

    GpuView(Context context, int surfaceId, int viewId, Reporter reporter) {
        super(context);
        this.surfaceId = surfaceId;
        this.viewId = viewId;
        this.reporter = reporter;
        setOpaque(false);
        setSurfaceTextureListener(this);
        activeViews.put(viewKey(surfaceId, viewId), this);
    }

    /** Configures swapchain color/depth formats and sample count from ItemGPU. */
    void configure(int colorFormat, int depthFormat, int sampleCount) {
        this.requestedColorFormat = colorFormat != FORMAT_UNDEFINED ? colorFormat : FORMAT_RGBA8_UNORM;
        this.requestedDepthFormat = depthFormat;
        this.requestedSampleCount = Math.max(sampleCount, 1);
    }

    /** Releases all GL resources and stops accepting frames when unmounted. */
    void destroy() {
        destroyed = true;
        activeViews.remove(viewKey(surfaceId, viewId), this);
        Handler gl = ShaderNode.glHandler();
        gl.post(() -> {
            activeSurfaceTexture = null;
            releaseSurfaceAndResources(false);
        });
    }

    private void enqueueFrame(byte[] payload) {
        if (destroyed) return;
        ShaderNode.glHandler().post(() -> executeFrame(payload));
    }

    @Override
    public void onSurfaceTextureAvailable(SurfaceTexture texture, int w, int h) {
        ShaderNode.glHandler().post(() -> {
            if (destroyed) {
                texture.release();
                return;
            }
            activeSurfaceTexture = texture;
            initSurface(texture, w, h, true);
        });
    }

    private void initSurface(SurfaceTexture texture, int w, int h, boolean retryOnLost) {
        if (destroyed || texture == null) return;
        if (ShaderNode.glFailed()) {
            emitError(generation, ERR_UNSUPPORTED, "OpenGL ES 3.0 context unavailable");
            return;
        }
        EGLDisplay display = ShaderNode.eglDisplay();
        EGLConfig config = ShaderNode.eglConfig();
        EGLContext context = ShaderNode.eglContext();
        if (display == null || config == null || context == null || context == EGL14.EGL_NO_CONTEXT) {
            emitError(generation, ERR_UNSUPPORTED, "OpenGL ES 3.0 EGL display or context not initialized");
            return;
        }
        EGLSurface s = EGL14.eglCreateWindowSurface(display, config, texture, new int[] {EGL14.EGL_NONE}, 0);
        if (s == null || s == EGL14.EGL_NO_SURFACE) {
            emitError(generation, ERR_FAILED, "eglCreateWindowSurface failed: 0x" + Integer.toHexString(EGL14.eglGetError()));
            return;
        }
        if (!EGL14.eglMakeCurrent(display, s, s, context)) {
            int err = EGL14.eglGetError();
            EGL14.eglDestroySurface(display, s);
            if (err == EGL14.EGL_CONTEXT_LOST) {
                ShaderNode.recreateContext();
                emitDeviceLost(generation, LOST_RESET, "EGL_CONTEXT_LOST");
                if (retryOnLost && !ShaderNode.glFailed()) {
                    initSurface(texture, w, h, false);
                }
            } else {
                emitError(generation, ERR_FAILED, "eglMakeCurrent failed: 0x" + Integer.toHexString(err));
            }
            return;
        }
        eglSurface = s;
        width = Math.max(w, 1);
        height = Math.max(h, 1);
        generation++;
        ready = true;

        int maxTex = Math.max(glInt(GLES30.GL_MAX_TEXTURE_SIZE, 4096), 2048);
        long maxUbo = Math.max(glInt(GLES30.GL_MAX_UNIFORM_BLOCK_SIZE, 16384), 16384);
        int maxAttribs = Math.max(glInt(GLES30.GL_MAX_VERTEX_ATTRIBS, 16), 16);
        int uboAlign = Math.max(glInt(GLES30.GL_UNIFORM_BUFFER_OFFSET_ALIGNMENT, 256), 1);
        maxSamples = Math.max(glInt(GLES30.GL_MAX_SAMPLES, 1), 1);
        String exts = GLES30.glGetString(GLES30.GL_EXTENSIONS);
        anisotropySupported = exts != null && exts.contains("GL_EXT_texture_filter_anisotropic");
        if (anisotropySupported) {
            float[] aniso = new float[1];
            GLES30.glGetFloatv(GL_MAX_TEXTURE_MAX_ANISOTROPY_EXT, aniso, 0);
            maxAnisotropy = Math.max(1f, aniso[0]);
        }
        boolean msaa4x = maxSamples >= 4;
        emitDeviceReady(generation, width, height, FORMAT_RGBA8_UNORM,
                maxTex, 256L << 20, maxUbo, 4, 8, maxAttribs, uboAlign,
                false, false, msaa4x, true, anisotropySupported);
    }

    @Override
    public void onSurfaceTextureSizeChanged(SurfaceTexture texture, int w, int h) {
        ShaderNode.glHandler().post(() -> {
            if (destroyed || !ready) return;
            width = Math.max(w, 1);
            height = Math.max(h, 1);
            emitResize(generation, width, height);
        });
    }

    @Override
    public boolean onSurfaceTextureDestroyed(SurfaceTexture texture) {
        ShaderNode.glHandler().post(() -> {
            if (activeSurfaceTexture == texture) activeSurfaceTexture = null;
            releaseSurfaceAndResources(!destroyed);
            texture.release();
        });
        return false;
    }

    @Override
    public void onSurfaceTextureUpdated(SurfaceTexture texture) {}

    /** Releases all GL objects and destroys the EGLSurface on the GL thread. */
    private void releaseSurfaceAndResources(boolean notifyLost) {
        EGLSurface s = eglSurface;
        boolean wasReady = ready;
        int lostGen = generation;
        eglSurface = null;
        ready = false;

        EGLDisplay display = ShaderNode.eglDisplay();
        EGLContext context = ShaderNode.eglContext();
        if (s != null && display != null && context != null && context != EGL14.EGL_NO_CONTEXT) {
            if (EGL14.eglMakeCurrent(display, s, s, context)) {
                deleteAllGlObjects();
                EGL14.eglMakeCurrent(display, EGL14.EGL_NO_SURFACE, EGL14.EGL_NO_SURFACE, EGL14.EGL_NO_CONTEXT);
            } else {
                clearObjectMaps();
            }
            EGL14.eglDestroySurface(display, s);
        } else {
            clearObjectMaps();
        }
        if (notifyLost && wasReady) {
            emitDeviceLost(lostGen, LOST_DESTROYED, "surface texture destroyed");
        }
    }

    private void deleteAllGlObjects() {
        if (!buffers.isEmpty()) {
            int[] ids = new int[buffers.size()];
            int i = 0;
            for (BufferObj b : buffers.values()) ids[i++] = b.glId;
            GLES30.glDeleteBuffers(ids.length, ids, 0);
        }
        if (!textures.isEmpty()) {
            int[] texIds = new int[textures.size()];
            int[] rbIds = new int[textures.size()];
            int tc = 0, rc = 0;
            for (TextureObj t : textures.values()) {
                if (t.isRenderbuffer) rbIds[rc++] = t.glId;
                else texIds[tc++] = t.glId;
            }
            if (tc > 0) GLES30.glDeleteTextures(tc, texIds, 0);
            if (rc > 0) GLES30.glDeleteRenderbuffers(rc, rbIds, 0);
        }
        if (!samplers.isEmpty()) {
            int[] ids = new int[samplers.size()];
            int i = 0;
            for (int id : samplers.values()) ids[i++] = id;
            GLES30.glDeleteSamplers(ids.length, ids, 0);
        }
        if (!pipelines.isEmpty()) {
            int[] vaos = new int[pipelines.size()];
            int i = 0;
            for (PipelineObj p : pipelines.values()) {
                if (p.program != 0) GLES30.glDeleteProgram(p.program);
                vaos[i++] = p.vao;
            }
            GLES30.glDeleteVertexArrays(vaos.length, vaos, 0);
        }
        int[] fbos = new int[3];
        int fboCount = 0;
        if (offscreenFbo != 0) { fbos[fboCount++] = offscreenFbo; offscreenFbo = 0; }
        if (readFbo != 0) { fbos[fboCount++] = readFbo; readFbo = 0; }
        if (drawFbo != 0) { fbos[fboCount++] = drawFbo; drawFbo = 0; }
        if (fboCount > 0) GLES30.glDeleteFramebuffers(fboCount, fbos, 0);
        clearObjectMaps();
    }

    private void clearObjectMaps() {
        buffers.clear();
        textures.clear();
        samplers.clear();
        bindGroupLayoutDynSlots.clear();
        bindGroups.clear();
        pipelines.clear();
        offscreenFbo = 0;
        readFbo = 0;
        drawFbo = 0;
        maxAttachedColors = 0;
    }

    /** Executes one encoded Protocol.EXT_GPU_FRAME on the shared GL thread. */
    private void executeFrame(byte[] payload) {
        if (destroyed || !ready || eglSurface == null || ShaderNode.glFailed()) return;
        EGLDisplay display = ShaderNode.eglDisplay();
        EGLContext context = ShaderNode.eglContext();
        if (display == null || context == null || context == EGL14.EGL_NO_CONTEXT) return;

        DataInputStream in = new DataInputStream(new ByteArrayInputStream(payload));
        int maxUnitUsed = 0;
        try {
            in.readInt(); // surfaceId
            in.readInt(); // viewId
            int frameGen = in.readInt();
            if (frameGen != 0 && frameGen != generation) return;
            boolean present = Protocol.flag(in.readUnsignedByte(), 0);

            if (!EGL14.eglMakeCurrent(display, eglSurface, eglSurface, context)) {
                if (EGL14.eglGetError() == EGL14.EGL_CONTEXT_LOST) {
                    handleContextLost();
                }
                return;
            }

            // 1. Retained resource ops (create, write, destroy, readback).
            int opCount = in.readInt();
            for (int i = 0; i < opCount; i++) {
                execResourceOp(in);
            }

            // 2. Copy ops outside render passes.
            int copyCount = in.readInt();
            for (int i = 0; i < copyCount; i++) {
                execCopyOp(in);
            }

            // 3. Render passes.
            int passCount = in.readInt();
            for (int i = 0; i < passCount; i++) {
                int units = execRenderPass(in);
                if (units > maxUnitUsed) maxUnitUsed = units;
            }

            // 4. Present swapchain if requested.
            if (present && eglSurface != null) {
                if (!EGL14.eglSwapBuffers(display, eglSurface)) {
                    if (EGL14.eglGetError() == EGL14.EGL_CONTEXT_LOST) {
                        handleContextLost();
                    }
                }
            }
        } catch (IOException e) {
            Log.w(TAG, "gpu: invalid frame payload", e);
            emitError(generation, ERR_FAILED, "invalid GPU frame: " + e.getMessage());
        } finally {
            restoreSharedContextState(maxUnitUsed);
        }
    }

    /** Restores default GL state so ShaderNode shares the EGL context cleanly. */
    private void restoreSharedContextState(int maxUnits) {
        if (!ready || eglSurface == null) return;
        GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, 0);
        GLES30.glBindVertexArray(0);
        GLES30.glBindBuffer(GLES30.GL_ARRAY_BUFFER, 0);
        GLES30.glBindBuffer(GLES30.GL_ELEMENT_ARRAY_BUFFER, 0);
        GLES30.glBindBuffer(GLES30.GL_UNIFORM_BUFFER, 0);
        GLES30.glBindBuffer(GLES30.GL_PIXEL_UNPACK_BUFFER, 0);
        GLES30.glUseProgram(0);
        GLES30.glDisable(GLES30.GL_DEPTH_TEST);
        GLES30.glDepthMask(true);
        GLES30.glDisable(GLES30.GL_CULL_FACE);
        GLES30.glDisable(GLES30.GL_BLEND);
        GLES30.glDisable(GLES30.GL_SCISSOR_TEST);
        GLES30.glDisable(GLES30.GL_POLYGON_OFFSET_FILL);
        GLES30.glColorMask(true, true, true, true);
        for (int u = 0; u < maxUnits; u++) {
            GLES30.glBindSampler(u, 0);
        }
        GLES30.glActiveTexture(GLES30.GL_TEXTURE0);
    }

    private void handleContextLost() {
        int lostGen = generation;
        ready = false;
        eglSurface = null;
        clearObjectMaps();
        ShaderNode.recreateContext();
        emitDeviceLost(lostGen, LOST_RESET, "EGL_CONTEXT_LOST");
        if (!destroyed && activeSurfaceTexture != null && !ShaderNode.glFailed()) {
            initSurface(activeSurfaceTexture, Math.max(width, 1), Math.max(height, 1), false);
        }
    }

    private void execResourceOp(DataInputStream in) throws IOException {
        int kind = in.readUnsignedByte();
        long id = in.readLong();
        switch (kind) {
            case OP_CREATE_BUFFER: {
                long size = in.readLong();
                int usage = in.readInt();
                byte[] data = Protocol.readBytes(in);
                int target = chooseBufferTarget(usage);
                int[] ids = new int[1];
                GLES30.glGenBuffers(1, ids, 0);
                int glId = ids[0];
                GLES30.glBindBuffer(target, glId);
                int hint = (usage & (BUF_COPY_DST | BUF_UNIFORM)) != 0
                        ? GLES30.GL_DYNAMIC_DRAW : GLES30.GL_STATIC_DRAW;
                if (data.length == size && data.length > 0) {
                    GLES30.glBufferData(target, (int) size, ByteBuffer.wrap(data), hint);
                } else {
                    GLES30.glBufferData(target, (int) size, null, hint);
                    if (data.length > 0) {
                        int n = (int) Math.min(data.length, size);
                        GLES30.glBufferSubData(target, 0, n, ByteBuffer.wrap(data, 0, n).slice());
                    }
                }
                GLES30.glBindBuffer(target, 0);
                BufferObj prev = buffers.put(id, new BufferObj(glId, target, size, usage));
                if (prev != null) GLES30.glDeleteBuffers(1, new int[] {prev.glId}, 0);
                break;
            }
            case OP_WRITE_BUFFER: {
                long offset = in.readLong();
                byte[] data = Protocol.readBytes(in);
                BufferObj buf = buffers.get(id);
                if (buf != null && data.length > 0 && offset >= 0 && offset + data.length <= buf.size) {
                    GLES30.glBindBuffer(buf.target, buf.glId);
                    GLES30.glBufferSubData(buf.target, (int) offset, data.length, ByteBuffer.wrap(data));
                    GLES30.glBindBuffer(buf.target, 0);
                }
                break;
            }
            case OP_DESTROY_BUFFER: {
                BufferObj prev = buffers.remove(id);
                if (prev != null) GLES30.glDeleteBuffers(1, new int[] {prev.glId}, 0);
                break;
            }
            case OP_CREATE_TEXTURE: {
                int dim = in.readUnsignedByte();
                int w = Math.max(in.readInt(), 1);
                int h = Math.max(in.readInt(), 1);
                int layers = Math.max(in.readInt(), 1);
                int mipLevels = Math.max(in.readInt(), 1);
                int sampleCount = Math.max(in.readInt(), 1);
                int format = in.readUnsignedByte();
                in.readInt(); // usage

                int[] fmt = mapTextureFormat(format);
                int internalFmt = fmt[0], pixFmt = fmt[1], pixType = fmt[2], bpp = fmt[3];
                boolean isDepth = fmt[4] != 0;

                TextureObj created;
                if (sampleCount > 1 && dim == DIM_2D && maxSamples > 1) {
                    int samples = Math.min(sampleCount, maxSamples);
                    int[] ids = new int[1];
                    GLES30.glGenRenderbuffers(1, ids, 0);
                    int glId = ids[0];
                    GLES30.glBindRenderbuffer(GLES30.GL_RENDERBUFFER, glId);
                    GLES30.glRenderbufferStorageMultisample(GLES30.GL_RENDERBUFFER, samples, internalFmt, w, h);
                    GLES30.glBindRenderbuffer(GLES30.GL_RENDERBUFFER, 0);
                    created = new TextureObj(glId, GLES30.GL_RENDERBUFFER, w, h, 1, 1, samples,
                            format, internalFmt, pixFmt, pixType, bpp, isDepth, true);
                } else {
                    int target = dim == DIM_CUBE ? GLES30.GL_TEXTURE_CUBE_MAP
                            : (dim == DIM_2D_ARRAY ? GLES30.GL_TEXTURE_2D_ARRAY : GLES30.GL_TEXTURE_2D);
                    int[] ids = new int[1];
                    GLES30.glGenTextures(1, ids, 0);
                    int glId = ids[0];
                    GLES30.glBindTexture(target, glId);
                    if (target == GLES30.GL_TEXTURE_2D_ARRAY) {
                        GLES30.glTexStorage3D(target, mipLevels, internalFmt, w, h, layers);
                    } else {
                        GLES30.glTexStorage2D(target, mipLevels, internalFmt, w, h);
                    }
                    int defaultFilter = isDepth ? GLES30.GL_NEAREST : GLES30.GL_LINEAR;
                    GLES30.glTexParameteri(target, GLES30.GL_TEXTURE_MIN_FILTER, defaultFilter);
                    GLES30.glTexParameteri(target, GLES30.GL_TEXTURE_MAG_FILTER, defaultFilter);
                    GLES30.glTexParameteri(target, GLES30.GL_TEXTURE_WRAP_S, GLES30.GL_CLAMP_TO_EDGE);
                    GLES30.glTexParameteri(target, GLES30.GL_TEXTURE_WRAP_T, GLES30.GL_CLAMP_TO_EDGE);
                    GLES30.glBindTexture(target, 0);
                    created = new TextureObj(glId, target, w, h, layers, mipLevels, 1,
                            format, internalFmt, pixFmt, pixType, bpp, isDepth, false);
                }

                TextureObj prev = textures.put(id, created);
                if (prev != null) deleteTextureObj(prev);
                break;
            }
            case OP_WRITE_TEXTURE: {
                int mipLevel = in.readInt();
                int arrayLayer = in.readInt();
                int originX = in.readInt();
                int originY = in.readInt();
                long dataOffset = in.readLong();
                int bytesPerRow = in.readInt();
                int rowsPerImage = in.readInt();
                byte[] data = Protocol.readBytes(in);

                TextureObj tex = textures.get(id);
                if (tex != null && !tex.isRenderbuffer && !tex.isDepth && tex.bpp > 0 && data.length > dataOffset) {
                    int mipW = Math.max(1, tex.width >> mipLevel);
                    int mipH = Math.max(1, tex.height >> mipLevel);
                    int availW = Math.max(0, mipW - originX);
                    int availH = Math.max(0, mipH - originY);
                    int off = (int) dataOffset;
                    int rem = data.length - off;
                    int rowStride = bytesPerRow > 0 ? bytesPerRow : availW * tex.bpp;
                    int uploadW = availW;
                    if (bytesPerRow > 0) {
                        uploadW = Math.min(availW, bytesPerRow / tex.bpp);
                    }
                    int uploadH = rowsPerImage > 0 ? Math.min(availH, rowsPerImage)
                            : (rowStride > 0 ? Math.min(availH, rem / rowStride) : availH);
                    if (uploadW > 0 && uploadH > 0) {
                        byte[] pixels = data;
                        int sliceOff = off;
                        if (tex.format == FORMAT_BGRA8_UNORM) {
                            pixels = swapRB(data, off, rem);
                            sliceOff = 0;
                        }
                        ByteBuffer buf = ByteBuffer.wrap(pixels, sliceOff, pixels.length - sliceOff).slice();
                        GLES30.glBindTexture(tex.target, tex.glId);
                        GLES30.glPixelStorei(GLES30.GL_UNPACK_ALIGNMENT, 1);
                        int rowPixels = (bytesPerRow > 0 && bytesPerRow != uploadW * tex.bpp)
                                ? bytesPerRow / tex.bpp : 0;
                        GLES30.glPixelStorei(GLES30.GL_UNPACK_ROW_LENGTH, rowPixels);
                        if (tex.target == GLES30.GL_TEXTURE_2D_ARRAY) {
                            GLES30.glTexSubImage3D(tex.target, mipLevel, originX, originY, arrayLayer,
                                    uploadW, uploadH, 1, tex.pixelFormat, tex.pixelType, buf);
                        } else if (tex.target == GLES30.GL_TEXTURE_CUBE_MAP) {
                            GLES30.glTexSubImage2D(GLES30.GL_TEXTURE_CUBE_MAP_POSITIVE_X + Math.min(arrayLayer, 5),
                                    mipLevel, originX, originY, uploadW, uploadH, tex.pixelFormat, tex.pixelType, buf);
                        } else {
                            GLES30.glTexSubImage2D(GLES30.GL_TEXTURE_2D, mipLevel, originX, originY,
                                    uploadW, uploadH, tex.pixelFormat, tex.pixelType, buf);
                        }
                        GLES30.glPixelStorei(GLES30.GL_UNPACK_ROW_LENGTH, 0);
                        GLES30.glBindTexture(tex.target, 0);
                    }
                }
                break;
            }
            case OP_DESTROY_TEXTURE: {
                TextureObj prev = textures.remove(id);
                if (prev != null) deleteTextureObj(prev);
                break;
            }
            case OP_CREATE_SAMPLER: {
                int minFilter = in.readUnsignedByte();
                int magFilter = in.readUnsignedByte();
                int mipmapFilter = in.readUnsignedByte();
                int addrU = in.readUnsignedByte();
                int addrV = in.readUnsignedByte();
                int addrW = in.readUnsignedByte();
                int compare = in.readUnsignedByte();
                int aniso = in.readUnsignedShort();

                int[] ids = new int[1];
                GLES30.glGenSamplers(1, ids, 0);
                int glId = ids[0];
                GLES30.glSamplerParameteri(glId, GLES30.GL_TEXTURE_MIN_FILTER, mapMinFilter(minFilter, mipmapFilter));
                GLES30.glSamplerParameteri(glId, GLES30.GL_TEXTURE_MAG_FILTER,
                        magFilter == FILTER_LINEAR ? GLES30.GL_LINEAR : GLES30.GL_NEAREST);
                GLES30.glSamplerParameteri(glId, GLES30.GL_TEXTURE_WRAP_S, mapAddressMode(addrU));
                GLES30.glSamplerParameteri(glId, GLES30.GL_TEXTURE_WRAP_T, mapAddressMode(addrV));
                GLES30.glSamplerParameteri(glId, GLES30.GL_TEXTURE_WRAP_R, mapAddressMode(addrW));
                if (compare != COMPARE_UNDEFINED) {
                    GLES30.glSamplerParameteri(glId, GLES30.GL_TEXTURE_COMPARE_MODE, GLES30.GL_COMPARE_REF_TO_TEXTURE);
                    GLES30.glSamplerParameteri(glId, GLES30.GL_TEXTURE_COMPARE_FUNC, mapCompare(compare));
                }
                if (anisotropySupported && aniso > 1) {
                    GLES30.glSamplerParameterf(glId, GL_TEXTURE_MAX_ANISOTROPY_EXT, Math.min(aniso, maxAnisotropy));
                }
                Integer prev = samplers.put(id, glId);
                if (prev != null) GLES30.glDeleteSamplers(1, new int[] {prev}, 0);
                break;
            }
            case OP_DESTROY_SAMPLER: {
                Integer prev = samplers.remove(id);
                if (prev != null) GLES30.glDeleteSamplers(1, new int[] {prev}, 0);
                break;
            }
            case OP_CREATE_BIND_GROUP_LAYOUT: {
                int entryCount = in.readInt();
                Map<Integer, Integer> dynSlots = new HashMap<>();
                int dynIndex = 0;
                for (int i = 0; i < entryCount; i++) {
                    int binding = in.readInt();
                    in.readUnsignedByte(); // visibility
                    in.readUnsignedByte(); // type
                    boolean hasDyn = Protocol.flag(in.readUnsignedByte(), 0);
                    if (hasDyn) dynSlots.put(binding, dynIndex++);
                }
                bindGroupLayoutDynSlots.put(id, dynSlots);
                break;
            }
            case OP_DESTROY_BIND_GROUP_LAYOUT:
                bindGroupLayoutDynSlots.remove(id);
                break;
            case OP_CREATE_BIND_GROUP: {
                long layoutId = in.readLong();
                int entryCount = in.readInt();
                BindGroupObj bg = new BindGroupObj(layoutId);
                for (int i = 0; i < entryCount; i++) {
                    int binding = in.readInt();
                    long bufId = in.readLong();
                    long offset = in.readLong();
                    long size = in.readLong();
                    long texId = in.readLong();
                    long sampId = in.readLong();
                    bg.entries.put(binding, new BindGroupEntryObj(binding, bufId, offset, size, texId, sampId));
                }
                bindGroups.put(id, bg);
                break;
            }
            case OP_DESTROY_BIND_GROUP:
                bindGroups.remove(id);
                break;
            case OP_CREATE_PIPELINE: {
                execCreatePipeline(id, in);
                break;
            }
            case OP_DESTROY_PIPELINE: {
                PipelineObj prev = pipelines.remove(id);
                if (prev != null) {
                    if (prev.program != 0) GLES30.glDeleteProgram(prev.program);
                    if (prev.vao != 0) GLES30.glDeleteVertexArrays(1, new int[] {prev.vao}, 0);
                }
                break;
            }
            case OP_READ_TEXTURE: {
                long readbackId = in.readLong();
                boolean readSurface = Protocol.flag(in.readUnsignedByte(), 0);
                int mipLevel = in.readInt();
                int arrayLayer = in.readInt();
                int rx = in.readInt();
                int ry = in.readInt();
                int rw = in.readInt();
                int rh = in.readInt();
                execReadTexture(id, readbackId, readSurface, mipLevel, arrayLayer, rx, ry, rw, rh);
                break;
            }
            default:
                throw new IOException("unknown GPU resource op " + kind);
        }
    }

    private void execCreatePipeline(long id, DataInputStream in) throws IOException {
        String vertSrc = Protocol.readString(in);
        String fragSrc = Protocol.readString(in);

        int ubCount = in.readInt();
        UniformBlockSlot[] ubs = new UniformBlockSlot[ubCount];
        for (int i = 0; i < ubCount; i++) {
            int group = in.readInt();
            int binding = in.readInt();
            String blockName = Protocol.readString(in);
            int slot = in.readInt();
            ubs[i] = new UniformBlockSlot(group, binding, blockName, slot);
        }

        int spCount = in.readInt();
        SamplerPair[] sps = new SamplerPair[spCount];
        for (int i = 0; i < spCount; i++) {
            String name = Protocol.readString(in);
            int unit = in.readInt();
            int texGroup = in.readInt();
            int texBinding = in.readInt();
            int sampGroup = in.readInt();
            int sampBinding = in.readInt();
            sps[i] = new SamplerPair(name, unit, texGroup, texBinding, sampGroup, sampBinding);
        }

        int vbCount = in.readInt();
        VertexBufferLayout[] vbs = new VertexBufferLayout[vbCount];
        for (int i = 0; i < vbCount; i++) {
            long stride = in.readLong();
            int stepMode = in.readUnsignedByte();
            int attrCount = in.readInt();
            VertexAttr[] attrs = new VertexAttr[attrCount];
            for (int j = 0; j < attrCount; j++) {
                int fmt = in.readUnsignedByte();
                long offset = in.readLong();
                int location = in.readInt();
                attrs[j] = new VertexAttr(fmt, offset, location);
            }
            vbs[i] = new VertexBufferLayout(stride, stepMode, attrs);
        }

        int topology = in.readUnsignedByte();
        int frontFace = in.readUnsignedByte();
        int cullMode = in.readUnsignedByte();

        int dsFlags = in.readUnsignedByte();
        boolean depthEnabled = Protocol.flag(dsFlags, 0);
        boolean depthWrite = Protocol.flag(dsFlags, 1);
        int glDepthCompare = GLES30.GL_LESS;
        int depthBias = 0;
        float depthBiasSlope = 0f;
        if (depthEnabled) {
            in.readUnsignedByte(); // depth format
            glDepthCompare = mapCompare(in.readUnsignedByte());
            depthBias = in.readInt();
            depthBiasSlope = in.readFloat();
        }

        int targetCount = in.readInt();
        int writeMask = 0x0F;
        boolean blendEnabled = false;
        int srcC = GLES30.GL_ONE, dstC = GLES30.GL_ZERO, opC = GLES30.GL_FUNC_ADD;
        int srcA = GLES30.GL_ONE, dstA = GLES30.GL_ZERO, opA = GLES30.GL_FUNC_ADD;
        for (int i = 0; i < targetCount; i++) {
            in.readUnsignedByte(); // format
            int mask = in.readUnsignedByte();
            boolean hasBlend = Protocol.flag(in.readUnsignedByte(), 0);
            int sc = GLES30.GL_ONE, dc = GLES30.GL_ZERO, oc = GLES30.GL_FUNC_ADD;
            int sa = GLES30.GL_ONE, da = GLES30.GL_ZERO, oa = GLES30.GL_FUNC_ADD;
            if (hasBlend) {
                sc = mapBlendFactor(in.readUnsignedByte());
                dc = mapBlendFactor(in.readUnsignedByte());
                oc = mapBlendOp(in.readUnsignedByte());
                sa = mapBlendFactor(in.readUnsignedByte());
                da = mapBlendFactor(in.readUnsignedByte());
                oa = mapBlendOp(in.readUnsignedByte());
            }
            if (i == 0) {
                writeMask = mask;
                blendEnabled = hasBlend;
                srcC = sc; dstC = dc; opC = oc;
                srcA = sa; dstA = da; opA = oa;
            }
        }
        in.readInt(); // sampleCount

        if (fragSrc.isEmpty()) {
            // Depth-only pass needs a minimal valid ES 3.00 fragment shader.
            fragSrc = "#version 300 es\nprecision highp float;\nvoid main() {}\n";
        }
        StringBuilder errLog = new StringBuilder();
        int vs = compileShader(GLES30.GL_VERTEX_SHADER, vertSrc, errLog);
        int fs = vs != 0 ? compileShader(GLES30.GL_FRAGMENT_SHADER, fragSrc, errLog) : 0;
        if (vs == 0 || fs == 0) {
            if (vs != 0) GLES30.glDeleteShader(vs);
            if (fs != 0) GLES30.glDeleteShader(fs);
            emitError(generation, ERR_SHADER_COMPILE, errLog.toString());
            return;
        }

        int prog = GLES30.glCreateProgram();
        GLES30.glAttachShader(prog, vs);
        GLES30.glAttachShader(prog, fs);
        GLES30.glLinkProgram(prog);
        GLES30.glDeleteShader(vs);
        GLES30.glDeleteShader(fs);

        int[] ok = new int[1];
        GLES30.glGetProgramiv(prog, GLES30.GL_LINK_STATUS, ok, 0);
        if (ok[0] == 0) {
            String info = GLES30.glGetProgramInfoLog(prog);
            Log.w(TAG, "gpu: program link error: " + info);
            GLES30.glDeleteProgram(prog);
            emitError(generation, ERR_SHADER_COMPILE, "program link failed: " + info);
            return;
        }

        GLES30.glUseProgram(prog);
        for (UniformBlockSlot ub : ubs) {
            int idx = GLES30.glGetUniformBlockIndex(prog, ub.blockName);
            if (idx != GLES30.GL_INVALID_INDEX) {
                GLES30.glUniformBlockBinding(prog, idx, ub.slot);
            }
        }
        for (SamplerPair sp : sps) {
            int loc = GLES30.glGetUniformLocation(prog, sp.name);
            if (loc >= 0) {
                GLES30.glUniform1i(loc, sp.unit);
            }
        }
        GLES30.glUseProgram(0);

        int[] vaos = new int[1];
        GLES30.glGenVertexArrays(1, vaos, 0);
        PipelineObj pipe = new PipelineObj(prog, vaos[0], ubs, sps, vbs,
                mapTopology(topology),
                frontFace == FRONT_CW ? GLES30.GL_CW : GLES30.GL_CCW,
                cullMode, depthEnabled, depthWrite, glDepthCompare, depthBias, depthBiasSlope,
                writeMask, blendEnabled, srcC, dstC, opC, srcA, dstA, opA);
        PipelineObj prev = pipelines.put(id, pipe);
        if (prev != null) {
            if (prev.program != 0) GLES30.glDeleteProgram(prev.program);
            if (prev.vao != 0) GLES30.glDeleteVertexArrays(1, new int[] {prev.vao}, 0);
        }
    }

    private void execReadTexture(long texId, long readbackId, boolean readSurface,
            int mipLevel, int arrayLayer, int rx, int ry, int rw, int rh) {
        int srcW, srcH;
        boolean flipY = true;
        if (readSurface) {
            GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, 0);
            srcW = width;
            srcH = height;
        } else {
            TextureObj tex = textures.get(texId);
            if (tex == null || tex.isDepth || tex.sampleCount > 1) {
                emitReadback(generation, readbackId, "texture not found or not directly readable", new byte[0]);
                return;
            }
            srcW = Math.max(1, tex.width >> mipLevel);
            srcH = Math.max(1, tex.height >> mipLevel);
            if (readFbo == 0) {
                int[] ids = new int[1];
                GLES30.glGenFramebuffers(1, ids, 0);
                readFbo = ids[0];
            }
            GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, readFbo);
            attachTextureToFbo(GLES30.GL_FRAMEBUFFER, GLES30.GL_COLOR_ATTACHMENT0, tex, mipLevel, arrayLayer);
            if (GLES30.glCheckFramebufferStatus(GLES30.GL_FRAMEBUFFER) != GLES30.GL_FRAMEBUFFER_COMPLETE) {
                GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, 0);
                emitReadback(generation, readbackId, "readback framebuffer incomplete", new byte[0]);
                return;
            }
        }
        int w = rw > 0 ? Math.min(rw, srcW - rx) : Math.max(0, srcW - rx);
        int h = rh > 0 ? Math.min(rh, srcH - ry) : Math.max(0, srcH - ry);
        if (w <= 0 || h <= 0) {
            GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, 0);
            emitReadback(generation, readbackId, "", new byte[0]);
            return;
        }
        int glY = Math.max(0, srcH - (ry + h));
        byte[] raw = new byte[w * h * 4];
        ByteBuffer buf = ByteBuffer.wrap(raw);
        GLES30.glPixelStorei(GLES30.GL_PACK_ALIGNMENT, 1);
        GLES30.glReadPixels(rx, glY, w, h, GLES30.GL_RGBA, GLES30.GL_UNSIGNED_BYTE, buf);
        GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, 0);

        if (flipY && h > 1) {
            int rowBytes = w * 4;
            byte[] tmp = new byte[rowBytes];
            for (int y = 0; y < h / 2; y++) {
                int top = y * rowBytes;
                int bot = (h - 1 - y) * rowBytes;
                System.arraycopy(raw, top, tmp, 0, rowBytes);
                System.arraycopy(raw, bot, raw, top, rowBytes);
                System.arraycopy(tmp, 0, raw, bot, rowBytes);
            }
        }
        emitReadback(generation, readbackId, "", raw);
    }

    private void execCopyOp(DataInputStream in) throws IOException {
        int kind = in.readUnsignedByte();
        long srcId = in.readLong();
        long dstId = in.readLong();
        switch (kind) {
            case COPY_BUFFER_TO_BUFFER: {
                long srcOff = in.readLong();
                long dstOff = in.readLong();
                long size = in.readLong();
                BufferObj src = buffers.get(srcId);
                BufferObj dst = buffers.get(dstId);
                if (src != null && dst != null && size > 0) {
                    GLES30.glBindBuffer(GLES30.GL_COPY_READ_BUFFER, src.glId);
                    GLES30.glBindBuffer(GLES30.GL_COPY_WRITE_BUFFER, dst.glId);
                    GLES30.glCopyBufferSubData(GLES30.GL_COPY_READ_BUFFER, GLES30.GL_COPY_WRITE_BUFFER,
                            (int) srcOff, (int) dstOff, (int) size);
                    GLES30.glBindBuffer(GLES30.GL_COPY_READ_BUFFER, 0);
                    GLES30.glBindBuffer(GLES30.GL_COPY_WRITE_BUFFER, 0);
                }
                break;
            }
            case COPY_BUFFER_TO_TEXTURE: {
                long srcOff = in.readLong();
                int bytesPerRow = in.readInt();
                int rowsPerImage = in.readInt();
                int mip = in.readInt();
                int layer = in.readInt();
                int ox = in.readInt();
                int oy = in.readInt();
                int w = in.readInt();
                int h = in.readInt();
                int layers = Math.max(in.readInt(), 1);
                BufferObj src = buffers.get(srcId);
                TextureObj dst = textures.get(dstId);
                if (src != null && dst != null && !dst.isRenderbuffer && !dst.isDepth && dst.bpp > 0 && w > 0 && h > 0) {
                    int rowBytes = bytesPerRow > 0 ? bytesPerRow : w * dst.bpp;
                    int copyLen = (int) Math.min(src.size - srcOff, (long) rowBytes * h * layers);
                    if (copyLen > 0) {
                        GLES30.glBindBuffer(GLES30.GL_COPY_READ_BUFFER, src.glId);
                        java.nio.Buffer mapped = GLES30.glMapBufferRange(
                                GLES30.GL_COPY_READ_BUFFER, (int) srcOff, copyLen, GLES30.GL_MAP_READ_BIT);
                        if (mapped != null) {
                            java.nio.Buffer upload = mapped;
                            if (dst.format == FORMAT_BGRA8_UNORM && mapped instanceof ByteBuffer) {
                                byte[] tmp = new byte[copyLen];
                                ((ByteBuffer) mapped).get(tmp);
                                upload = ByteBuffer.wrap(swapRB(tmp, 0, tmp.length));
                            }
                            GLES30.glBindTexture(dst.target, dst.glId);
                            GLES30.glPixelStorei(GLES30.GL_UNPACK_ALIGNMENT, 1);
                            int rowPixels = (bytesPerRow > 0 && bytesPerRow != w * dst.bpp) ? bytesPerRow / dst.bpp : 0;
                            GLES30.glPixelStorei(GLES30.GL_UNPACK_ROW_LENGTH, rowPixels);
                            GLES30.glPixelStorei(GLES30.GL_UNPACK_IMAGE_HEIGHT, rowsPerImage > 0 ? rowsPerImage : 0);
                            if (dst.target == GLES30.GL_TEXTURE_2D_ARRAY) {
                                GLES30.glTexSubImage3D(dst.target, mip, ox, oy, layer, w, h, layers,
                                        dst.pixelFormat, dst.pixelType, upload);
                            } else if (dst.target == GLES30.GL_TEXTURE_CUBE_MAP) {
                                GLES30.glTexSubImage2D(GLES30.GL_TEXTURE_CUBE_MAP_POSITIVE_X + Math.min(layer, 5),
                                        mip, ox, oy, w, h, dst.pixelFormat, dst.pixelType, upload);
                            } else {
                                GLES30.glTexSubImage2D(GLES30.GL_TEXTURE_2D, mip, ox, oy, w, h,
                                        dst.pixelFormat, dst.pixelType, upload);
                            }
                            GLES30.glPixelStorei(GLES30.GL_UNPACK_ROW_LENGTH, 0);
                            GLES30.glPixelStorei(GLES30.GL_UNPACK_IMAGE_HEIGHT, 0);
                            GLES30.glBindTexture(dst.target, 0);
                            GLES30.glUnmapBuffer(GLES30.GL_COPY_READ_BUFFER);
                        }
                        GLES30.glBindBuffer(GLES30.GL_COPY_READ_BUFFER, 0);
                    }
                }
                break;
            }
            case COPY_TEXTURE_TO_TEXTURE: {
                int srcMip = in.readInt();
                int srcLayer = in.readInt();
                int sx = in.readInt();
                int sy = in.readInt();
                int dstMip = in.readInt();
                int dstLayer = in.readInt();
                int dx = in.readInt();
                int dy = in.readInt();
                int w = in.readInt();
                int h = in.readInt();
                int layers = Math.max(in.readInt(), 1);
                TextureObj src = textures.get(srcId);
                TextureObj dst = textures.get(dstId);
                if (src != null && dst != null && w > 0 && h > 0) {
                    if (readFbo == 0) {
                        int[] ids = new int[1];
                        GLES30.glGenFramebuffers(1, ids, 0);
                        readFbo = ids[0];
                    }
                    if (drawFbo == 0) {
                        int[] ids = new int[1];
                        GLES30.glGenFramebuffers(1, ids, 0);
                        drawFbo = ids[0];
                    }
                    int attachment = src.isDepth ? GLES30.GL_DEPTH_ATTACHMENT : GLES30.GL_COLOR_ATTACHMENT0;
                    int mask = src.isDepth ? GLES30.GL_DEPTH_BUFFER_BIT : GLES30.GL_COLOR_BUFFER_BIT;
                    for (int l = 0; l < layers; l++) {
                        GLES30.glBindFramebuffer(GLES30.GL_READ_FRAMEBUFFER, readFbo);
                        attachTextureToFbo(GLES30.GL_READ_FRAMEBUFFER, attachment, src, srcMip, srcLayer + l);
                        GLES30.glBindFramebuffer(GLES30.GL_DRAW_FRAMEBUFFER, drawFbo);
                        attachTextureToFbo(GLES30.GL_DRAW_FRAMEBUFFER, attachment, dst, dstMip, dstLayer + l);
                        GLES30.glBlitFramebuffer(sx, sy, sx + w, sy + h, dx, dy, dx + w, dy + h,
                                mask, GLES30.GL_NEAREST);
                    }
                    GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, 0);
                }
                break;
            }
            default:
                throw new IOException("unknown GPU copy op " + kind);
        }
    }

    private int execRenderPass(DataInputStream in) throws IOException {
        int colorCount = in.readInt();
        long[] colorTexIds = new long[colorCount];
        long[] resolveTexIds = new long[colorCount];
        int[] colorMips = new int[colorCount];
        int[] colorLayers = new int[colorCount];
        int[] colorLoadOps = new int[colorCount];
        int[] colorClears = new int[colorCount];
        for (int i = 0; i < colorCount; i++) {
            colorTexIds[i] = in.readLong();
            resolveTexIds[i] = in.readLong();
            colorMips[i] = in.readInt();
            colorLayers[i] = in.readInt();
            colorLoadOps[i] = in.readUnsignedByte();
            in.readUnsignedByte(); // storeOp
            colorClears[i] = in.readInt();
        }

        boolean hasDepth = Protocol.flag(in.readUnsignedByte(), 0);
        long depthTexId = 0;
        int depthLoadOp = 0;
        float clearDepth = 1f;
        int stencilLoadOp = 0;
        int clearStencil = 0;
        if (hasDepth) {
            depthTexId = in.readLong();
            depthLoadOp = in.readUnsignedByte();
            in.readUnsignedByte(); // depthStoreOp
            clearDepth = in.readFloat();
            stencilLoadOp = in.readUnsignedByte();
            in.readUnsignedByte(); // stencilStoreOp
            clearStencil = in.readInt();
        }

        boolean offscreen = (colorCount > 0 && colorTexIds[0] != 0) || (hasDepth && depthTexId != 0);
        int passW = width;
        int passH = height;

        if (!offscreen) {
            GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, 0);
        } else {
            if (offscreenFbo == 0) {
                int[] ids = new int[1];
                GLES30.glGenFramebuffers(1, ids, 0);
                offscreenFbo = ids[0];
            }
            GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, offscreenFbo);
            int[] drawBufs = new int[Math.max(colorCount, 1)];
            int attachedColors = 0;
            for (int i = 0; i < colorCount; i++) {
                TextureObj tex = textures.get(colorTexIds[i]);
                if (tex != null) {
                    attachTextureToFbo(GLES30.GL_FRAMEBUFFER, GLES30.GL_COLOR_ATTACHMENT0 + i,
                            tex, colorMips[i], colorLayers[i]);
                    drawBufs[i] = GLES30.GL_COLOR_ATTACHMENT0 + i;
                    attachedColors++;
                    if (i == 0) {
                        passW = Math.max(1, tex.width >> colorMips[i]);
                        passH = Math.max(1, tex.height >> colorMips[i]);
                    }
                } else {
                    GLES30.glFramebufferTexture2D(GLES30.GL_FRAMEBUFFER, GLES30.GL_COLOR_ATTACHMENT0 + i,
                            GLES30.GL_TEXTURE_2D, 0, 0);
                    drawBufs[i] = GLES30.GL_NONE;
                }
            }
            for (int i = colorCount; i < maxAttachedColors; i++) {
                GLES30.glFramebufferTexture2D(GLES30.GL_FRAMEBUFFER, GLES30.GL_COLOR_ATTACHMENT0 + i,
                        GLES30.GL_TEXTURE_2D, 0, 0);
            }
            maxAttachedColors = Math.max(maxAttachedColors, colorCount);
            if (attachedColors == 0) {
                drawBufs[0] = GLES30.GL_NONE;
                GLES30.glDrawBuffers(1, drawBufs, 0);
            } else {
                GLES30.glDrawBuffers(colorCount, drawBufs, 0);
            }
            GLES30.glFramebufferTexture2D(GLES30.GL_FRAMEBUFFER, GLES30.GL_DEPTH_STENCIL_ATTACHMENT,
                    GLES30.GL_TEXTURE_2D, 0, 0);
            if (hasDepth && depthTexId != 0) {
                TextureObj dtex = textures.get(depthTexId);
                if (dtex != null) {
                    int att = dtex.format == FORMAT_DEPTH24_PLUS_STENCIL8
                            ? GLES30.GL_DEPTH_STENCIL_ATTACHMENT : GLES30.GL_DEPTH_ATTACHMENT;
                    attachTextureToFbo(GLES30.GL_FRAMEBUFFER, att, dtex, 0, 0);
                    if (attachedColors == 0) {
                        passW = dtex.width;
                        passH = dtex.height;
                    }
                }
            }
        }

        // Default pass viewport, scissor, and clear ops.
        GLES30.glViewport(0, 0, passW, passH);
        GLES30.glDepthRangef(0f, 1f);
        GLES30.glDisable(GLES30.GL_SCISSOR_TEST);
        GLES30.glColorMask(true, true, true, true);
        GLES30.glDepthMask(true);

        if (colorCount == 1 && colorLoadOps[0] == LOAD_CLEAR) {
            int c = colorClears[0];
            GLES30.glClearColor(((c >>> 24) & 0xff) / 255f, ((c >>> 16) & 0xff) / 255f,
                    ((c >>> 8) & 0xff) / 255f, (c & 0xff) / 255f);
            int mask = GLES30.GL_COLOR_BUFFER_BIT;
            if (hasDepth && depthLoadOp == LOAD_CLEAR) {
                GLES30.glClearDepthf(clearDepth);
                mask |= GLES30.GL_DEPTH_BUFFER_BIT;
            }
            if (hasDepth && stencilLoadOp == LOAD_CLEAR) {
                GLES30.glClearStencil(clearStencil);
                mask |= GLES30.GL_STENCIL_BUFFER_BIT;
            }
            GLES30.glClear(mask);
        } else {
            for (int i = 0; i < colorCount; i++) {
                if (colorLoadOps[i] == LOAD_CLEAR) {
                    int c = colorClears[i];
                    float[] rgba = {((c >>> 24) & 0xff) / 255f, ((c >>> 16) & 0xff) / 255f,
                            ((c >>> 8) & 0xff) / 255f, (c & 0xff) / 255f};
                    GLES30.glClearBufferfv(GLES30.GL_COLOR, i, rgba, 0);
                }
            }
            int dsMask = 0;
            if (hasDepth && depthLoadOp == LOAD_CLEAR) {
                GLES30.glClearDepthf(clearDepth);
                dsMask |= GLES30.GL_DEPTH_BUFFER_BIT;
            }
            if (hasDepth && stencilLoadOp == LOAD_CLEAR) {
                GLES30.glClearStencil(clearStencil);
                dsMask |= GLES30.GL_STENCIL_BUFFER_BIT;
            }
            if (dsMask != 0) GLES30.glClear(dsMask);
        }

        // Pass dynamic binding state.
        PipelineObj activePipe = null;
        BindGroupObj[] boundGroups = new BindGroupObj[8];
        int[][] boundDynOffsets = new int[8][];
        BufferObj[] boundVBs = new BufferObj[16];
        long[] boundVBOffsets = new long[16];
        BufferObj boundIB = null;
        int boundIBFormat = INDEX_UINT16;
        long boundIBOffset = 0;
        boolean bindingsDirty = true;
        boolean vertexDirty = true;
        int maxUnitUsed = 0;

        int cmdCount = in.readInt();
        for (int i = 0; i < cmdCount; i++) {
            int cmd = in.readUnsignedByte();
            switch (cmd) {
                case CMD_SET_VIEWPORT: {
                    float vx = in.readFloat();
                    float vy = in.readFloat();
                    float vw = in.readFloat();
                    float vh = in.readFloat();
                    float minD = in.readFloat();
                    float maxD = in.readFloat();
                    int glY = Math.round(passH - (vy + vh));
                    GLES30.glViewport(Math.round(vx), glY, Math.round(vw), Math.round(vh));
                    GLES30.glDepthRangef(minD, maxD);
                    break;
                }
                case CMD_SET_SCISSOR: {
                    int sx = in.readInt();
                    int sy = in.readInt();
                    int sw = in.readInt();
                    int sh = in.readInt();
                    int glY = passH - (sy + sh);
                    GLES30.glEnable(GLES30.GL_SCISSOR_TEST);
                    GLES30.glScissor(sx, glY, Math.max(sw, 0), Math.max(sh, 0));
                    break;
                }
                case CMD_SET_PIPELINE: {
                    long pipeId = in.readLong();
                    activePipe = pipelines.get(pipeId);
                    if (activePipe != null) {
                        applyPipelineState(activePipe);
                        bindingsDirty = true;
                        vertexDirty = true;
                    }
                    break;
                }
                case CMD_SET_BIND_GROUP: {
                    int slot = in.readInt();
                    long bgId = in.readLong();
                    int dynCount = in.readInt();
                    int[] dyn = dynCount > 0 ? new int[dynCount] : null;
                    for (int d = 0; d < dynCount; d++) dyn[d] = in.readInt();
                    if (slot >= 0 && slot < boundGroups.length) {
                        boundGroups[slot] = bindGroups.get(bgId);
                        boundDynOffsets[slot] = dyn;
                        bindingsDirty = true;
                    }
                    break;
                }
                case CMD_SET_VERTEX_BUFFER: {
                    int slot = in.readInt();
                    long bufId = in.readLong();
                    long offset = in.readLong();
                    if (slot >= 0 && slot < boundVBs.length) {
                        boundVBs[slot] = buffers.get(bufId);
                        boundVBOffsets[slot] = offset;
                        vertexDirty = true;
                    }
                    break;
                }
                case CMD_SET_INDEX_BUFFER: {
                    long bufId = in.readLong();
                    boundIBFormat = in.readUnsignedByte();
                    boundIBOffset = in.readLong();
                    boundIB = buffers.get(bufId);
                    break;
                }
                case CMD_SET_STENCIL_REFERENCE: {
                    in.readInt();
                    break;
                }
                case CMD_SET_BLEND_CONSTANT: {
                    float r = in.readFloat();
                    float g = in.readFloat();
                    float b = in.readFloat();
                    float a = in.readFloat();
                    GLES30.glBlendColor(r, g, b, a);
                    break;
                }
                case CMD_DRAW: {
                    int vertexCount = in.readInt();
                    int firstVertex = in.readInt();
                    if (activePipe != null && vertexCount > 0) {
                        if (vertexDirty) {
                            bindVertexBuffers(activePipe, boundVBs, boundVBOffsets, 0, 0);
                            vertexDirty = false;
                        }
                        if (bindingsDirty) {
                            maxUnitUsed = Math.max(maxUnitUsed, bindResources(activePipe, boundGroups, boundDynOffsets));
                            bindingsDirty = false;
                        }
                        GLES30.glDrawArrays(activePipe.glTopology, firstVertex, vertexCount);
                    }
                    break;
                }
                case CMD_DRAW_INDEXED: {
                    int indexCount = in.readInt();
                    int firstIndex = in.readInt();
                    int baseVertex = in.readInt();
                    if (activePipe != null && boundIB != null && indexCount > 0) {
                        if (vertexDirty || baseVertex != 0) {
                            bindVertexBuffers(activePipe, boundVBs, boundVBOffsets, baseVertex, 0);
                            vertexDirty = baseVertex != 0;
                        }
                        if (bindingsDirty) {
                            maxUnitUsed = Math.max(maxUnitUsed, bindResources(activePipe, boundGroups, boundDynOffsets));
                            bindingsDirty = false;
                        }
                        GLES30.glBindBuffer(GLES30.GL_ELEMENT_ARRAY_BUFFER, boundIB.glId);
                        int type = boundIBFormat == INDEX_UINT32 ? GLES30.GL_UNSIGNED_INT : GLES30.GL_UNSIGNED_SHORT;
                        int elemBytes = boundIBFormat == INDEX_UINT32 ? 4 : 2;
                        GLES30.glDrawElements(activePipe.glTopology, indexCount, type,
                                (int) (boundIBOffset + (long) firstIndex * elemBytes));
                    }
                    break;
                }
                case CMD_DRAW_INSTANCED: {
                    int vertexCount = in.readInt();
                    int instanceCount = in.readInt();
                    int firstVertex = in.readInt();
                    int firstInstance = in.readInt();
                    if (activePipe != null && vertexCount > 0 && instanceCount > 0) {
                        if (vertexDirty || firstInstance != 0) {
                            bindVertexBuffers(activePipe, boundVBs, boundVBOffsets, 0, firstInstance);
                            vertexDirty = firstInstance != 0;
                        }
                        if (bindingsDirty) {
                            maxUnitUsed = Math.max(maxUnitUsed, bindResources(activePipe, boundGroups, boundDynOffsets));
                            bindingsDirty = false;
                        }
                        GLES30.glDrawArraysInstanced(activePipe.glTopology, firstVertex, vertexCount, instanceCount);
                    }
                    break;
                }
                case CMD_DRAW_INDEXED_INSTANCED: {
                    int indexCount = in.readInt();
                    int instanceCount = in.readInt();
                    int firstIndex = in.readInt();
                    int firstInstance = in.readInt();
                    int baseVertex = in.readInt();
                    if (activePipe != null && boundIB != null && indexCount > 0 && instanceCount > 0) {
                        if (vertexDirty || baseVertex != 0 || firstInstance != 0) {
                            bindVertexBuffers(activePipe, boundVBs, boundVBOffsets, baseVertex, firstInstance);
                            vertexDirty = baseVertex != 0 || firstInstance != 0;
                        }
                        if (bindingsDirty) {
                            maxUnitUsed = Math.max(maxUnitUsed, bindResources(activePipe, boundGroups, boundDynOffsets));
                            bindingsDirty = false;
                        }
                        GLES30.glBindBuffer(GLES30.GL_ELEMENT_ARRAY_BUFFER, boundIB.glId);
                        int type = boundIBFormat == INDEX_UINT32 ? GLES30.GL_UNSIGNED_INT : GLES30.GL_UNSIGNED_SHORT;
                        int elemBytes = boundIBFormat == INDEX_UINT32 ? 4 : 2;
                        GLES30.glDrawElementsInstanced(activePipe.glTopology, indexCount, type,
                                (int) (boundIBOffset + (long) firstIndex * elemBytes), instanceCount);
                    }
                    break;
                }
                default:
                    throw new IOException("unknown GPU render command " + cmd);
            }
        }
        if (offscreen) {
            for (int i = 0; i < colorCount; i++) {
                if (resolveTexIds[i] == 0) continue;
                TextureObj dst = textures.get(resolveTexIds[i]);
                if (dst == null) continue;
                if (drawFbo == 0) {
                    int[] ids = new int[1];
                    GLES30.glGenFramebuffers(1, ids, 0);
                    drawFbo = ids[0];
                }
                GLES30.glDisable(GLES30.GL_SCISSOR_TEST);
                GLES30.glBindFramebuffer(GLES30.GL_READ_FRAMEBUFFER, offscreenFbo);
                GLES30.glReadBuffer(GLES30.GL_COLOR_ATTACHMENT0 + i);
                GLES30.glBindFramebuffer(GLES30.GL_DRAW_FRAMEBUFFER, drawFbo);
                attachTextureToFbo(GLES30.GL_DRAW_FRAMEBUFFER, GLES30.GL_COLOR_ATTACHMENT0, dst, 0, 0);
                GLES30.glBlitFramebuffer(0, 0, passW, passH, 0, 0, dst.width, dst.height,
                        GLES30.GL_COLOR_BUFFER_BIT, GLES30.GL_NEAREST);
            }
            GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, 0);
        }
        return maxUnitUsed;
    }

    private void applyPipelineState(PipelineObj p) {
        GLES30.glUseProgram(p.program);
        GLES30.glBindVertexArray(p.vao);
        GLES30.glFrontFace(p.glFrontFace);
        if (p.cullMode == CULL_NONE) {
            GLES30.glDisable(GLES30.GL_CULL_FACE);
        } else {
            GLES30.glEnable(GLES30.GL_CULL_FACE);
            GLES30.glCullFace(p.cullMode == CULL_FRONT ? GLES30.GL_FRONT : GLES30.GL_BACK);
        }
        if (p.depthEnabled) {
            GLES30.glEnable(GLES30.GL_DEPTH_TEST);
            GLES30.glDepthMask(p.depthWrite);
            GLES30.glDepthFunc(p.glDepthCompare);
            if (p.depthBias != 0 || p.depthBiasSlopeScale != 0f) {
                GLES30.glEnable(GLES30.GL_POLYGON_OFFSET_FILL);
                GLES30.glPolygonOffset(p.depthBiasSlopeScale, p.depthBias);
            } else {
                GLES30.glDisable(GLES30.GL_POLYGON_OFFSET_FILL);
            }
        } else {
            GLES30.glDisable(GLES30.GL_DEPTH_TEST);
            GLES30.glDepthMask(false);
            GLES30.glDisable(GLES30.GL_POLYGON_OFFSET_FILL);
        }
        GLES30.glColorMask((p.writeMask & 1) != 0, (p.writeMask & 2) != 0,
                (p.writeMask & 4) != 0, (p.writeMask & 8) != 0);
        if (p.blendEnabled) {
            GLES30.glEnable(GLES30.GL_BLEND);
            GLES30.glBlendFuncSeparate(p.srcColor, p.dstColor, p.srcAlpha, p.dstAlpha);
            GLES30.glBlendEquationSeparate(p.opColor, p.opAlpha);
        } else {
            GLES30.glDisable(GLES30.GL_BLEND);
        }
    }

    private void bindVertexBuffers(PipelineObj p, BufferObj[] vbs, long[] offsets, int baseVertex, int firstInstance) {
        GLES30.glBindVertexArray(p.vao);
        for (int slot = 0; slot < p.vertexBuffers.length && slot < vbs.length; slot++) {
            VertexBufferLayout layout = p.vertexBuffers[slot];
            BufferObj vb = vbs[slot];
            if (vb == null) continue;
            GLES30.glBindBuffer(GLES30.GL_ARRAY_BUFFER, vb.glId);
            long baseOffset = offsets[slot];
            if (layout.stepMode == STEP_INSTANCE) {
                baseOffset += (long) firstInstance * layout.stride;
            } else {
                baseOffset += (long) baseVertex * layout.stride;
            }
            int divisor = layout.stepMode == STEP_INSTANCE ? 1 : 0;
            for (VertexAttr a : layout.attrs) {
                GLES30.glEnableVertexAttribArray(a.location);
                int attrOff = (int) (baseOffset + a.offset);
                int stride = (int) layout.stride;
                switch (a.format) {
                    case VTX_FLOAT32:
                        GLES30.glVertexAttribPointer(a.location, 1, GLES30.GL_FLOAT, false, stride, attrOff);
                        break;
                    case VTX_FLOAT32X2:
                        GLES30.glVertexAttribPointer(a.location, 2, GLES30.GL_FLOAT, false, stride, attrOff);
                        break;
                    case VTX_FLOAT32X3:
                        GLES30.glVertexAttribPointer(a.location, 3, GLES30.GL_FLOAT, false, stride, attrOff);
                        break;
                    case VTX_FLOAT32X4:
                        GLES30.glVertexAttribPointer(a.location, 4, GLES30.GL_FLOAT, false, stride, attrOff);
                        break;
                    case VTX_UNORM8X4:
                        GLES30.glVertexAttribPointer(a.location, 4, GLES30.GL_UNSIGNED_BYTE, true, stride, attrOff);
                        break;
                    case VTX_SINT32:
                        GLES30.glVertexAttribIPointer(a.location, 1, GLES30.GL_INT, stride, attrOff);
                        break;
                    case VTX_SINT32X2:
                        GLES30.glVertexAttribIPointer(a.location, 2, GLES30.GL_INT, stride, attrOff);
                        break;
                    case VTX_SINT32X3:
                        GLES30.glVertexAttribIPointer(a.location, 3, GLES30.GL_INT, stride, attrOff);
                        break;
                    case VTX_SINT32X4:
                        GLES30.glVertexAttribIPointer(a.location, 4, GLES30.GL_INT, stride, attrOff);
                        break;
                    case VTX_UINT32:
                        GLES30.glVertexAttribIPointer(a.location, 1, GLES30.GL_UNSIGNED_INT, stride, attrOff);
                        break;
                    case VTX_UINT32X2:
                        GLES30.glVertexAttribIPointer(a.location, 2, GLES30.GL_UNSIGNED_INT, stride, attrOff);
                        break;
                    case VTX_UINT32X3:
                        GLES30.glVertexAttribIPointer(a.location, 3, GLES30.GL_UNSIGNED_INT, stride, attrOff);
                        break;
                    case VTX_UINT32X4:
                        GLES30.glVertexAttribIPointer(a.location, 4, GLES30.GL_UNSIGNED_INT, stride, attrOff);
                        break;
                    default:
                        break;
                }
                GLES30.glVertexAttribDivisor(a.location, divisor);
            }
        }
    }

    private int bindResources(PipelineObj p, BindGroupObj[] groups, int[][] dynOffsets) {
        for (UniformBlockSlot ub : p.uniformBlocks) {
            if (ub.group < 0 || ub.group >= groups.length) continue;
            BindGroupObj bg = groups[ub.group];
            if (bg == null) continue;
            BindGroupEntryObj entry = bg.entries.get(ub.binding);
            if (entry == null || entry.bufferId == 0) continue;
            BufferObj buf = buffers.get(entry.bufferId);
            if (buf == null) continue;
            long off = entry.offset;
            if (dynOffsets[ub.group] != null) {
                Map<Integer, Integer> dynSlots = bindGroupLayoutDynSlots.get(bg.layoutId);
                Integer dynIdx = dynSlots != null ? dynSlots.get(ub.binding) : null;
                if (dynIdx != null && dynIdx >= 0 && dynIdx < dynOffsets[ub.group].length) {
                    off += dynOffsets[ub.group][dynIdx] & 0xffffffffL;
                }
            }
            long sz = entry.size > 0 ? entry.size : (buf.size - off);
            if (off == 0 && sz >= buf.size) {
                GLES30.glBindBufferBase(GLES30.GL_UNIFORM_BUFFER, ub.slot, buf.glId);
            } else {
                GLES30.glBindBufferRange(GLES30.GL_UNIFORM_BUFFER, ub.slot, buf.glId, (int) off, (int) sz);
            }
        }
        int maxUnits = 0;
        for (SamplerPair sp : p.samplerPairs) {
            GLES30.glActiveTexture(GLES30.GL_TEXTURE0 + sp.unit);
            int texGlId = 0;
            int texTarget = GLES30.GL_TEXTURE_2D;
            if (sp.texGroup >= 0 && sp.texGroup < groups.length && groups[sp.texGroup] != null) {
                BindGroupEntryObj te = groups[sp.texGroup].entries.get(sp.texBinding);
                if (te != null && te.textureId != 0) {
                    TextureObj tex = textures.get(te.textureId);
                    if (tex != null) {
                        texGlId = tex.glId;
                        texTarget = tex.target;
                    }
                }
            }
            GLES30.glBindTexture(texTarget, texGlId);

            int sampGlId = 0;
            if (sp.sampGroup >= 0 && sp.sampGroup < groups.length && groups[sp.sampGroup] != null) {
                BindGroupEntryObj se = groups[sp.sampGroup].entries.get(sp.sampBinding);
                if (se != null && se.samplerId != 0) {
                    Integer sId = samplers.get(se.samplerId);
                    if (sId != null) sampGlId = sId;
                }
            }
            GLES30.glBindSampler(sp.unit, sampGlId);
            if (sp.unit + 1 > maxUnits) maxUnits = sp.unit + 1;
        }
        return maxUnits;
    }

    // --- Event emission back to Go (Protocol.GPU_EVENT) ---

    private void emitDeviceReady(int gen, int w, int h, int surfaceFmt,
            int maxTex, long maxBuf, long maxUbo, int maxBindGroups, int maxVBs, int maxAttrs, int uboAlign,
            boolean storageBuffers, boolean compute, boolean msaa4x, boolean depth32f, boolean aniso) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeInt(surfaceId);
            d.writeInt(viewId);
            d.writeByte(EVENT_DEVICE_READY);
            d.writeInt(gen);
            d.writeInt(w);
            d.writeInt(h);
            d.writeByte(surfaceFmt);
            d.writeInt(maxTex);
            d.writeLong(maxBuf);
            d.writeLong(maxUbo);
            d.writeInt(maxBindGroups);
            d.writeInt(maxVBs);
            d.writeInt(maxAttrs);
            d.writeInt(uboAlign);
            int feats = (storageBuffers ? 1 : 0) | (compute ? 2 : 0) | (msaa4x ? 4 : 0)
                    | (depth32f ? 8 : 0) | (aniso ? 16 : 0);
            d.writeByte(feats);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        reporter.send(bytes.toByteArray());
    }

    private void emitResize(int gen, int w, int h) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeInt(surfaceId);
            d.writeInt(viewId);
            d.writeByte(EVENT_RESIZE);
            d.writeInt(gen);
            d.writeInt(w);
            d.writeInt(h);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        reporter.send(bytes.toByteArray());
    }

    private void emitDeviceLost(int gen, int reason, String message) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeInt(surfaceId);
            d.writeInt(viewId);
            d.writeByte(EVENT_DEVICE_LOST);
            d.writeInt(gen);
            d.writeByte(reason);
            Protocol.writeString(d, message == null ? "" : message);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        reporter.send(bytes.toByteArray());
    }

    private void emitError(int gen, int errorCode, String message) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeInt(surfaceId);
            d.writeInt(viewId);
            d.writeByte(EVENT_ERROR);
            d.writeInt(gen);
            d.writeByte(errorCode);
            Protocol.writeString(d, message == null ? "" : message);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        reporter.send(bytes.toByteArray());
    }

    private void emitReadback(int gen, long readbackId, String errorMessage, byte[] pixels) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream(32 + pixels.length);
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeInt(surfaceId);
            d.writeInt(viewId);
            d.writeByte(EVENT_READBACK);
            d.writeInt(gen);
            d.writeLong(readbackId);
            Protocol.writeString(d, errorMessage == null ? "" : errorMessage);
            d.writeInt(pixels.length);
            d.write(pixels);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        reporter.send(bytes.toByteArray());
    }

    // --- OpenGL ES 3.0 helpers & enum mappings ---

    private static int glInt(int pname, int fallback) {
        int[] v = new int[1];
        GLES30.glGetIntegerv(pname, v, 0);
        return v[0] > 0 ? v[0] : fallback;
    }

    private static int compileShader(int type, String source, StringBuilder errOut) {
        int sh = GLES30.glCreateShader(type);
        GLES30.glShaderSource(sh, source);
        GLES30.glCompileShader(sh);
        int[] ok = new int[1];
        GLES30.glGetShaderiv(sh, GLES30.GL_COMPILE_STATUS, ok, 0);
        if (ok[0] == 0) {
            String info = GLES30.glGetShaderInfoLog(sh);
            Log.w(TAG, "gpu: shader compile error: " + info);
            errOut.append(type == GLES30.GL_VERTEX_SHADER ? "vertex: " : "fragment: ").append(info);
            GLES30.glDeleteShader(sh);
            return 0;
        }
        return sh;
    }

    private static void deleteTextureObj(TextureObj tex) {
        if (tex.isRenderbuffer) {
            GLES30.glDeleteRenderbuffers(1, new int[] {tex.glId}, 0);
        } else {
            GLES30.glDeleteTextures(1, new int[] {tex.glId}, 0);
        }
    }

    private static void attachTextureToFbo(int fboTarget, int attachment, TextureObj tex, int mip, int layer) {
        if (tex.isRenderbuffer) {
            GLES30.glFramebufferRenderbuffer(fboTarget, attachment, GLES30.GL_RENDERBUFFER, tex.glId);
        } else if (tex.target == GLES30.GL_TEXTURE_2D_ARRAY) {
            GLES30.glFramebufferTextureLayer(fboTarget, attachment, tex.glId, mip, layer);
        } else if (tex.target == GLES30.GL_TEXTURE_CUBE_MAP) {
            GLES30.glFramebufferTexture2D(fboTarget, attachment,
                    GLES30.GL_TEXTURE_CUBE_MAP_POSITIVE_X + Math.min(layer, 5), tex.glId, mip);
        } else {
            GLES30.glFramebufferTexture2D(fboTarget, attachment, GLES30.GL_TEXTURE_2D, tex.glId, mip);
        }
    }

    private static int chooseBufferTarget(int usage) {
        if ((usage & BUF_INDEX) != 0 && (usage & (BUF_VERTEX | BUF_UNIFORM)) == 0) {
            return GLES30.GL_ELEMENT_ARRAY_BUFFER;
        }
        if ((usage & BUF_UNIFORM) != 0) {
            return GLES30.GL_UNIFORM_BUFFER;
        }
        return GLES30.GL_ARRAY_BUFFER;
    }

    private static int[] mapTextureFormat(int fmt) {
        // {internalFormat, pixelFormat, pixelType, bytesPerPixel, isDepth}
        switch (fmt) {
            case FORMAT_RGBA8_UNORM_SRGB:
                return new int[] {GLES30.GL_SRGB8_ALPHA8, GLES30.GL_RGBA, GLES30.GL_UNSIGNED_BYTE, 4, 0};
            case FORMAT_RG8_UNORM:
                return new int[] {GLES30.GL_RG8, GLES30.GL_RG, GLES30.GL_UNSIGNED_BYTE, 2, 0};
            case FORMAT_R8_UNORM:
                return new int[] {GLES30.GL_R8, GLES30.GL_RED, GLES30.GL_UNSIGNED_BYTE, 1, 0};
            case FORMAT_RGBA16_FLOAT:
                return new int[] {GLES30.GL_RGBA16F, GLES30.GL_RGBA, GLES30.GL_HALF_FLOAT, 8, 0};
            case FORMAT_DEPTH24_PLUS:
                return new int[] {GLES30.GL_DEPTH_COMPONENT24, GLES30.GL_DEPTH_COMPONENT, GLES30.GL_UNSIGNED_INT, 0, 1};
            case FORMAT_DEPTH24_PLUS_STENCIL8:
                return new int[] {GLES30.GL_DEPTH24_STENCIL8, GLES30.GL_DEPTH_STENCIL, GLES30.GL_UNSIGNED_INT_24_8, 0, 1};
            case FORMAT_DEPTH32_FLOAT:
                return new int[] {GLES30.GL_DEPTH_COMPONENT32F, GLES30.GL_DEPTH_COMPONENT, GLES30.GL_FLOAT, 0, 1};
            case FORMAT_RGBA8_UNORM:
            case FORMAT_BGRA8_UNORM:
            default:
                return new int[] {GLES30.GL_RGBA8, GLES30.GL_RGBA, GLES30.GL_UNSIGNED_BYTE, 4, 0};
        }
    }

    private static byte[] swapRB(byte[] src, int offset, int len) {
        byte[] out = new byte[len];
        for (int i = 0; i + 3 < len; i += 4) {
            out[i] = src[offset + i + 2];
            out[i + 1] = src[offset + i + 1];
            out[i + 2] = src[offset + i];
            out[i + 3] = src[offset + i + 3];
        }
        return out;
    }

    private static int mapMinFilter(int minF, int mipF) {
        if (minF == FILTER_LINEAR) {
            return mipF == FILTER_LINEAR ? GLES30.GL_LINEAR_MIPMAP_LINEAR : GLES30.GL_LINEAR_MIPMAP_NEAREST;
        }
        return mipF == FILTER_LINEAR ? GLES30.GL_NEAREST_MIPMAP_LINEAR : GLES30.GL_NEAREST_MIPMAP_NEAREST;
    }

    private static int mapAddressMode(int mode) {
        switch (mode) {
            case ADDR_REPEAT: return GLES30.GL_REPEAT;
            case ADDR_MIRROR_REPEAT: return GLES30.GL_MIRRORED_REPEAT;
            default: return GLES30.GL_CLAMP_TO_EDGE;
        }
    }

    private static int mapCompare(int cmp) {
        switch (cmp) {
            case COMPARE_NEVER: return GLES30.GL_NEVER;
            case COMPARE_LESS: return GLES30.GL_LESS;
            case COMPARE_EQUAL: return GLES30.GL_EQUAL;
            case COMPARE_LESS_EQUAL: return GLES30.GL_LEQUAL;
            case COMPARE_GREATER: return GLES30.GL_GREATER;
            case COMPARE_NOT_EQUAL: return GLES30.GL_NOTEQUAL;
            case COMPARE_GREATER_EQUAL: return GLES30.GL_GEQUAL;
            case COMPARE_ALWAYS: return GLES30.GL_ALWAYS;
            default: return GLES30.GL_LESS;
        }
    }

    private static int mapTopology(int topo) {
        switch (topo) {
            case TOPO_TRIANGLE_STRIP: return GLES30.GL_TRIANGLE_STRIP;
            case TOPO_LINE_LIST: return GLES30.GL_LINES;
            case TOPO_LINE_STRIP: return GLES30.GL_LINE_STRIP;
            case TOPO_POINT_LIST: return GLES30.GL_POINTS;
            default: return GLES30.GL_TRIANGLES;
        }
    }

    private static int mapBlendFactor(int f) {
        switch (f) {
            case BLEND_ZERO: return GLES30.GL_ZERO;
            case BLEND_ONE: return GLES30.GL_ONE;
            case BLEND_SRC: return GLES30.GL_SRC_COLOR;
            case BLEND_ONE_MINUS_SRC: return GLES30.GL_ONE_MINUS_SRC_COLOR;
            case BLEND_SRC_ALPHA: return GLES30.GL_SRC_ALPHA;
            case BLEND_ONE_MINUS_SRC_ALPHA: return GLES30.GL_ONE_MINUS_SRC_ALPHA;
            case BLEND_DST: return GLES30.GL_DST_COLOR;
            case BLEND_ONE_MINUS_DST: return GLES30.GL_ONE_MINUS_DST_COLOR;
            case BLEND_DST_ALPHA: return GLES30.GL_DST_ALPHA;
            case BLEND_ONE_MINUS_DST_ALPHA: return GLES30.GL_ONE_MINUS_DST_ALPHA;
            case BLEND_CONSTANT: return GLES30.GL_CONSTANT_COLOR;
            case BLEND_ONE_MINUS_CONSTANT: return GLES30.GL_ONE_MINUS_CONSTANT_COLOR;
            default: return GLES30.GL_ONE;
        }
    }

    private static int mapBlendOp(int op) {
        switch (op) {
            case BLEND_SUBTRACT: return GLES30.GL_FUNC_SUBTRACT;
            case BLEND_REVERSE_SUBTRACT: return GLES30.GL_FUNC_REVERSE_SUBTRACT;
            case BLEND_MIN: return GLES30.GL_MIN;
            case BLEND_MAX: return GLES30.GL_MAX;
            default: return GLES30.GL_FUNC_ADD;
        }
    }
}
