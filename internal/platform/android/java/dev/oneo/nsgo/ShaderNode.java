package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.SurfaceTexture;
import android.opengl.EGL14;
import android.opengl.EGLConfig;
import android.opengl.EGLContext;
import android.opengl.EGLDisplay;
import android.opengl.EGLSurface;
import android.opengl.GLES30;
import android.opengl.GLUtils;
import android.os.Handler;
import android.os.HandlerThread;
import android.util.Log;
import android.view.TextureView;
import java.io.DataInputStream;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.HashMap;
import java.util.Map;

/**
 * A shader view: a TextureView the program Go translated into GLSL ES
 * draws into with OpenGL ES 3.0, as android/shader.go writes it. Every
 * shader view shares one EGL context on a GL thread of its own, which
 * compiles each program once; the UI thread hands over the uniform block
 * of the latest frame and never waits for the GPU. It draws when its lanes,
 * its size or an image change, never on a timer.
 */
final class ShaderNode extends TextureView implements TextureView.SurfaceTextureListener {
    private static final String TAG = "nsgo";

    /** A program as Go translated it: its fragment stage and where its uniform lanes lie. */
    static final class Program {
        final String source;
        final int[] offsets;
        final byte[] types;
        final int size, images;
        // In the GL thread's context.
        int id = -1, height = -1, ubo, vao;

        Program(String source, int[] offsets, byte[] types, int size, int images) {
            this.source = source;
            this.offsets = offsets;
            this.types = types;
            this.size = size;
            this.images = images;
        }
    }

    /** The programs Go sent, by its numbers; the UI thread writes it, the GL thread reads it. */
    private static final Map<Integer, Program> programs = new java.util.concurrent.ConcurrentHashMap<>();

    // What a lane holds, as android/shader.go's lane constants.
    private static final int LANE_FLOAT = 0, LANE_INT = 1, LANE_UINT = 2, LANE_BOOL = 3;

    private static HandlerThread thread;
    private static Handler gl;
    private static EGLDisplay display;
    private static EGLContext context;
    private static EGLConfig config;
    private static boolean failed;

    private Program program;
    private double[] lanes = new double[0], values;
    private int[] nodeLanes = new int[0];
    private String[] images = new String[0];
    private final float density;
    /** The animated nodes the lanes follow, for NsgoView to bind. */
    int[] boundNodes = new int[0];

    // Shared with the GL thread under the node's lock.
    private EGLSurface surface;
    private int width, height;
    private ByteBuffer frame;
    private Program frameProgram;
    private String[] frameImages;
    private Bitmap[] frameBitmaps;
    private boolean pending;
    // The GL thread's: the images' textures.
    private final Map<String, Integer> textures = new HashMap<>();

    ShaderNode(Context context) {
        super(context);
        density = context.getResources().getDisplayMetrics().density;
        setOpaque(false);
        setSurfaceTextureListener(this);
    }

    /** Reads an ItemShader as shader.go's writeShader writes it, and draws. */
    void set(DataInputStream in) throws IOException {
        int id = in.readInt();
        if ((in.readUnsignedByte() & 1) != 0) {
            String source = Protocol.readString(in);
            int n = in.readInt();
            int[] offsets = new int[n];
            byte[] types = new byte[n];
            for (int i = 0; i < n; i++) {
                offsets[i] = in.readInt();
                types[i] = in.readByte();
            }
            int size = in.readInt(), images = in.readInt();
            programs.put(id, new Program(source, offsets, types, size, images));
        }
        program = programs.get(id);
        lanes = new double[in.readInt()];
        for (int i = 0; i < lanes.length; i++) lanes[i] = in.readDouble();
        int nodes = in.readInt();
        nodeLanes = new int[nodes];
        int[] nodeIds = new int[nodes];
        for (int i = 0; i < nodes; i++) {
            nodeLanes[i] = in.readInt();
            nodeIds[i] = in.readInt();
        }
        images = new String[in.readInt()];
        for (int i = 0; i < images.length; i++) images[i] = Protocol.readString(in);
        boundNodes = nodeIds;
        values = null;
        draw();
    }

    /** Draws again with the bound nodes' values, in boundNodes' order, in their lanes. */
    void patch(double[] values) {
        this.values = values.clone();
        draw();
    }

    /** Hands the GL thread the uniform block of the latest frame, and has it draw. */
    void draw() {
        Program p = program;
        if (p == null || p.source.isEmpty()) return;
        double[] all = new double[3 + 2 * p.images + lanes.length];
        System.arraycopy(lanes, 0, all, 3 + 2 * p.images, lanes.length);
        if (values != null) {
            for (int i = 0; i < nodeLanes.length && i < values.length; i++) {
                int lane = nodeLanes[i];
                if (lane >= 0 && lane < lanes.length) all[3 + 2 * p.images + lane] = values[i];
            }
        }
        float w = getWidth() / density, h = getHeight() / density;
        all[0] = w;
        all[1] = h;
        all[2] = density;
        Bitmap[] bitmaps = new Bitmap[images.length];
        for (int i = 0; i < images.length; i++) {
            CanvasNode.Picture pic = CanvasNode.pictures.get(images[i]);
            if (pic == null) continue;
            bitmaps[i] = pic.bitmap;
            if (i < p.images) {
                all[3 + 2 * i] = pic.bitmap.getWidth();
                all[4 + 2 * i] = pic.bitmap.getHeight();
            }
        }
        ByteBuffer block = ByteBuffer.allocateDirect(Math.max(p.size, 16)).order(ByteOrder.LITTLE_ENDIAN);
        for (int i = 0; i < all.length && i < p.offsets.length; i++) {
            int at = p.offsets[i];
            double v = all[i];
            switch (p.types[i]) {
                case LANE_FLOAT: block.putFloat(at, (float) v); break;
                case LANE_INT: block.putInt(at, (int) v); break;
                case LANE_UINT: block.putInt(at, (int) (long) v); break;
                case LANE_BOOL: block.putInt(at, v != 0 ? 1 : 0); break;
                default: break;
            }
        }
        synchronized (this) {
            frame = block;
            frameProgram = p;
            frameImages = images.clone();
            frameBitmaps = bitmaps;
            width = getWidth();
            height = getHeight();
            if (pending) return;
            pending = true;
        }
        start();
        gl.post(this::render);
    }

    /** Starts the GL thread and its context, once. */
    private static synchronized void start() {
        if (thread != null) return;
        thread = new HandlerThread("nsgo-shaders");
        thread.start();
        gl = new Handler(thread.getLooper());
        gl.post(ShaderNode::makeContext);
    }

    private static void makeContext() {
        display = EGL14.eglGetDisplay(EGL14.EGL_DEFAULT_DISPLAY);
        int[] version = new int[2];
        if (!EGL14.eglInitialize(display, version, 0, version, 1)) {
            failed = true;
            return;
        }
        int[] attribs = {EGL14.EGL_RENDERABLE_TYPE, 0x40 /* EGL_OPENGL_ES3_BIT */, EGL14.EGL_RED_SIZE, 8,
                EGL14.EGL_GREEN_SIZE, 8, EGL14.EGL_BLUE_SIZE, 8, EGL14.EGL_ALPHA_SIZE, 8, EGL14.EGL_NONE};
        EGLConfig[] configs = new EGLConfig[1];
        int[] n = new int[1];
        if (!EGL14.eglChooseConfig(display, attribs, 0, configs, 0, 1, n, 0) || n[0] == 0) {
            failed = true;
            return;
        }
        config = configs[0];
        context = EGL14.eglCreateContext(display, config, EGL14.EGL_NO_CONTEXT,
                new int[] {EGL14.EGL_CONTEXT_CLIENT_VERSION, 3, EGL14.EGL_NONE}, 0);
        failed = context == EGL14.EGL_NO_CONTEXT;
        if (failed) Log.w(TAG, "shader: no OpenGL ES 3.0 context");
    }

    @Override
    public void onSurfaceTextureAvailable(SurfaceTexture texture, int w, int h) {
        start();
        gl.post(() -> {
            if (failed) return;
            EGLSurface s = EGL14.eglCreateWindowSurface(display, config, texture, new int[] {EGL14.EGL_NONE}, 0);
            synchronized (this) {
                surface = s;
            }
        });
        draw();
    }

    @Override
    public void onSurfaceTextureSizeChanged(SurfaceTexture texture, int w, int h) {
        draw();
    }

    @Override
    public boolean onSurfaceTextureDestroyed(SurfaceTexture texture) {
        // The surface goes on the GL thread, then the texture with it.
        gl.post(() -> {
            EGLSurface s;
            synchronized (this) {
                s = surface;
                surface = null;
            }
            if (s != null) {
                // The textures go with it; images come back when it does.
                if (!textures.isEmpty() && EGL14.eglMakeCurrent(display, s, s, context)) {
                    int[] ids = new int[textures.size()];
                    int i = 0;
                    for (int id : textures.values()) ids[i++] = id;
                    GLES30.glDeleteTextures(ids.length, ids, 0);
                }
                textures.clear();
                EGL14.eglMakeCurrent(display, EGL14.EGL_NO_SURFACE, EGL14.EGL_NO_SURFACE, EGL14.EGL_NO_CONTEXT);
                EGL14.eglDestroySurface(display, s);
            }
            texture.release();
        });
        return false;
    }

    @Override
    public void onSurfaceTextureUpdated(SurfaceTexture texture) {}

    /** Draws the latest frame, on the GL thread. */
    private void render() {
        EGLSurface s;
        ByteBuffer block;
        String[] imgs;
        Bitmap[] bitmaps;
        int w, h;
        Program p;
        synchronized (this) {
            pending = false;
            s = surface;
            block = frame;
            p = frameProgram;
            imgs = frameImages;
            bitmaps = frameBitmaps;
            w = width;
            h = height;
        }
        if (failed || s == null || block == null || p == null || w <= 0 || h <= 0) return;
        if (!EGL14.eglMakeCurrent(display, s, s, context)) return;
        if (p.id == -1 && !link(p)) p.id = 0;
        GLES30.glViewport(0, 0, w, h);
        GLES30.glClearColor(0, 0, 0, 0);
        GLES30.glClear(GLES30.GL_COLOR_BUFFER_BIT);
        if (p.id > 0) {
            GLES30.glUseProgram(p.id);
            GLES30.glUniform1f(p.height, h);
            for (int i = 0; i < p.images; i++) {
                GLES30.glActiveTexture(GLES30.GL_TEXTURE0 + i);
                GLES30.glBindTexture(GLES30.GL_TEXTURE_2D, i < imgs.length ? texture(imgs[i], bitmaps[i]) : 0);
            }
            GLES30.glBindBuffer(GLES30.GL_UNIFORM_BUFFER, p.ubo);
            block.position(0);
            GLES30.glBufferSubData(GLES30.GL_UNIFORM_BUFFER, 0, block.capacity(), block);
            GLES30.glBindBufferBase(GLES30.GL_UNIFORM_BUFFER, 0, p.ubo);
            GLES30.glBindVertexArray(p.vao);
            GLES30.glDrawArrays(GLES30.GL_TRIANGLES, 0, 3);
        }
        EGL14.eglSwapBuffers(display, s);
    }

    /** Returns image key's texture, made from bitmap once it decoded; 0 until then. */
    private int texture(String key, Bitmap bitmap) {
        Integer t = textures.get(key);
        if (t != null) return t;
        if (bitmap == null) return 0;
        int[] id = new int[1];
        GLES30.glGenTextures(1, id, 0);
        GLES30.glBindTexture(GLES30.GL_TEXTURE_2D, id[0]);
        // Bitmaps are premultiplied, as shaders read colors.
        GLUtils.texImage2D(GLES30.GL_TEXTURE_2D, 0, bitmap, 0);
        GLES30.glTexParameteri(GLES30.GL_TEXTURE_2D, GLES30.GL_TEXTURE_MIN_FILTER, GLES30.GL_NEAREST);
        GLES30.glTexParameteri(GLES30.GL_TEXTURE_2D, GLES30.GL_TEXTURE_MAG_FILTER, GLES30.GL_NEAREST);
        GLES30.glTexParameteri(GLES30.GL_TEXTURE_2D, GLES30.GL_TEXTURE_WRAP_S, GLES30.GL_CLAMP_TO_EDGE);
        GLES30.glTexParameteri(GLES30.GL_TEXTURE_2D, GLES30.GL_TEXTURE_WRAP_T, GLES30.GL_CLAMP_TO_EDGE);
        textures.put(key, id[0]);
        return id[0];
    }

    private static final String VERTEX = "#version 300 es\n"
            + "void main() {\n"
            + "\tvec2 uv = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));\n"
            + "\tgl_Position = vec4(uv * 2.0 - 1.0, 0.0, 1.0);\n"
            + "}\n";

    /** Compiles and links p in the shared context, reporting whether it can draw. */
    private static boolean link(Program p) {
        int vs = compile(GLES30.GL_VERTEX_SHADER, VERTEX), fs = compile(GLES30.GL_FRAGMENT_SHADER, p.source);
        if (vs == 0 || fs == 0) return false;
        int prog = GLES30.glCreateProgram();
        GLES30.glAttachShader(prog, vs);
        GLES30.glAttachShader(prog, fs);
        GLES30.glLinkProgram(prog);
        GLES30.glDeleteShader(vs);
        GLES30.glDeleteShader(fs);
        int[] ok = new int[1];
        GLES30.glGetProgramiv(prog, GLES30.GL_LINK_STATUS, ok, 0);
        if (ok[0] == 0) {
            Log.w(TAG, "shader: GLES: " + GLES30.glGetProgramInfoLog(prog));
            GLES30.glDeleteProgram(prog);
            return false;
        }
        p.id = prog;
        p.height = GLES30.glGetUniformLocation(prog, "nsgoHeight");
        GLES30.glUseProgram(prog);
        for (int i = 0; i < p.images; i++) GLES30.glUniform1i(GLES30.glGetUniformLocation(prog, "T" + i), i);
        int block = GLES30.glGetUniformBlockIndex(prog, "NsgoUniforms");
        if (block != GLES30.GL_INVALID_INDEX) GLES30.glUniformBlockBinding(prog, block, 0);
        int[] ids = new int[1];
        GLES30.glGenBuffers(1, ids, 0);
        p.ubo = ids[0];
        GLES30.glBindBuffer(GLES30.GL_UNIFORM_BUFFER, p.ubo);
        GLES30.glBufferData(GLES30.GL_UNIFORM_BUFFER, Math.max(p.size, 16), null, GLES30.GL_DYNAMIC_DRAW);
        GLES30.glGenVertexArrays(1, ids, 0);
        p.vao = ids[0];
        return true;
    }

    private static int compile(int kind, String source) {
        int sh = GLES30.glCreateShader(kind);
        GLES30.glShaderSource(sh, source);
        GLES30.glCompileShader(sh);
        int[] ok = new int[1];
        GLES30.glGetShaderiv(sh, GLES30.GL_COMPILE_STATUS, ok, 0);
        if (ok[0] == 0) {
            Log.w(TAG, "shader: GLES: " + GLES30.glGetShaderInfoLog(sh));
            GLES30.glDeleteShader(sh);
            return 0;
        }
        return sh;
    }
}
