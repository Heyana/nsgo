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
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * A shader view: a TextureView the program Go translated into GLSL ES
 * draws into with OpenGL ES 3.0, as android/shader.go writes it. Every
 * shader view shares one EGL context on a GL thread of its own, which
 * compiles each program once; the UI thread hands over the uniform block
 * of the latest frame and never waits for the GPU. It draws when its lanes,
 * its size or an image change, never on a timer. An image of pixels comes
 * with the item, its bytes only when Go copied a new version, and goes into
 * a texture the view keeps and updates in place.
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

    /** The programs Go sent, by its numbers, until it releases them; the UI thread writes it, the GL thread reads it. */
    private static final Map<Integer, Program> programs = new java.util.concurrent.ConcurrentHashMap<>();
    /** The GL thread's: the programs released, until the shared context is current to delete them in. */
    private static final List<Program> released = new ArrayList<>();

    // What a lane holds, as android/shader.go's lane constants.
    private static final int LANE_FLOAT = 0, LANE_INT = 1, LANE_UINT = 2, LANE_BOOL = 3;
    // What an image is, as android/shader.go's shaderImage constants.
    private static final int IMAGE_KEY = 0, IMAGE_PIXELS = 1;

    /** A version of an image's pixels: premultiplied RGBA, top row first. */
    static final class Pixels {
        final ByteBuffer bytes;
        final int width, height;

        Pixels(ByteBuffer bytes, int width, int height) {
            this.bytes = bytes;
            this.width = width;
            this.height = height;
        }
    }

    private static HandlerThread thread;
    private static Handler gl;
    private static EGLDisplay display;
    private static EGLContext context;
    private static EGLConfig config;
    private static EGLSurface pictureSurface;
    private static int emptyPicture;
    private static boolean failed;

    private Program program;
    private double[] lanes = new double[0], values;
    private int[] nodeLanes = new int[0];
    private byte[] nodeChannels = new byte[0];
    // Role and picture slots are as in driver.Shader.
    static final int FILL = 0, EFFECT = 1, TRANSITION = 2, SCREEN = 3, SCREEN_TRANSITION = 4, NAV = 5;
    static final int CONTENT = 0, BACKDROP = 1, FROM = 2, TO = 3;
    int role, reads, margin;
    boolean running, backwards;
    private ShaderPicture[] inputs;
    private int requiredPictures, generation;
    private volatile int shownGeneration = -1;
    Runnable pictureShown;

    boolean available() {
        return program != null && !program.source.isEmpty();
    }

    boolean effectReady() {
        return shownGeneration == generation;
    }

    void resetEffectFrame() {
        generation++;
    }

    void inputs(ShaderPicture[] inputs, int required) {
        this.inputs = inputs != null ? inputs.clone() : null;
        requiredPictures = required;
        draw();
    }
    private String[] images = new String[0];
    /** The latest pixels of the images that are pixels, by their place; null for the others. */
    private Pixels[] pixels = new Pixels[0];
    private final float density;
    /** The animated nodes the lanes follow, for NsgoView to bind. */
    int[] boundNodes = new int[0];

    // Shared with the GL thread under the node's lock.
    private EGLSurface surface;
    private int width, height;
    private ByteBuffer frame;
    private Program frameProgram;
    private String[] frameImages;
    private Pixels[] framePixels;
    private ShaderPicture[] frameInputs;
    private int frameRequired, frameGeneration;
    private Bitmap[] frameBitmaps;
    private List<CanvasImages<CanvasNode.Picture>.Lease> frameLeases;
    private final CanvasImages<CanvasNode.Picture> pictures;
    private boolean pending;
    // The GL thread's: the images' textures, and those of pixels by their place with the pixels each holds.
    private final Map<String, Integer> textures = new LinkedHashMap<>(64, .75f, true);
    private final int[] pixelTextures = new int[4];
    private final Pixels[] uploaded = new Pixels[4];

    ShaderNode(Context context, CanvasImages<CanvasNode.Picture> pictures) {
        super(context);
        this.pictures = pictures;
        density = context.getResources().getDisplayMetrics().density;
        setOpaque(false);
        setImportantForAccessibility(IMPORTANT_FOR_ACCESSIBILITY_NO);
        setSurfaceTextureListener(this);
    }

    /** Reads an ItemShader as shader.go's writeShader writes it, and draws. */
    void set(DataInputStream in) throws IOException {
        int id = in.readInt();
        if (id == 0) {
            program = null;
            boundNodes = new int[0];
            return;
        }
        role = in.readUnsignedByte();
        reads = in.readUnsignedByte();
        int flags = in.readUnsignedByte();
        running = Protocol.flag(flags, 0);
        backwards = Protocol.flag(flags, 1);
        margin = Math.round((float) in.readDouble() * density);
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
        Program next = programs.get(id);
        if (program != next) resetEffectFrame();
        program = next;
        lanes = new double[in.readInt()];
        for (int i = 0; i < lanes.length; i++) lanes[i] = in.readDouble();
        int nodes = in.readInt();
        nodeLanes = new int[nodes];
        nodeChannels = new byte[nodes];
        int[] nodeIds = new int[nodes];
        for (int i = 0; i < nodes; i++) {
            nodeLanes[i] = in.readInt();
            nodeIds[i] = in.readInt();
            nodeChannels[i] = in.readByte();
        }
        int count = in.readInt();
        String[] keys = new String[count];
        Pixels[] px = new Pixels[count];
        for (int i = 0; i < count; i++) {
            int kind = in.readUnsignedByte();
            if (kind == IMAGE_KEY) {
                keys[i] = Protocol.readString(in);
            } else if (kind == IMAGE_PIXELS) {
                int w = in.readInt(), h = in.readInt();
                if ((in.readUnsignedByte() & 1) != 0) {
                    byte[] bytes = new byte[w * h * 4];
                    in.readFully(bytes);
                    px[i] = new Pixels(ByteBuffer.wrap(bytes), w, h);
                } else if (i < pixels.length && pixels[i] != null && pixels[i].width == w && pixels[i].height == h) {
                    // The same version as before.
                    px[i] = pixels[i];
                }
            }
        }
        images = keys;
        pixels = px;
        boundNodes = nodeIds;
        values = null;
    }

    /** Draws again with the bound nodes' values, in boundNodes' order, in their lanes. */
    void patch(double[] numbers, int[] colors) {
        values = numbers.clone();
        for (int i = 0; i < nodeChannels.length && i < values.length; i++) {
            int channel = nodeChannels[i];
            // Animator holds colors in RGBA order, most significant byte first.
            if (channel != 0) values[i] = ((colors[i] >>> (32 - 8 * channel)) & 0xff) / 255.0;
        }
        draw();
    }

    /** Hands the GL thread the uniform block of the latest frame, and has it draw. */
    void draw() {
        Program p = program;
        if (!available() || role != FILL && inputs == null) return;
        double[] all = new double[3 + 2 * p.images + lanes.length];
        System.arraycopy(lanes, 0, all, 3 + 2 * p.images, lanes.length);
        if (values != null) {
            for (int i = 0; i < nodeLanes.length && i < values.length; i++) {
                int lane = nodeLanes[i];
                if (lane >= 0 && lane < lanes.length) all[3 + 2 * p.images + lane] = values[i];
            }
        }
        if (inputs != null && lanes.length >= 3) {
            // Android lays out the padding in whole pixels.
            all[3 + 2 * p.images + 2] = margin / density;
            for (int i = 0; i < p.images; i++) {
                ShaderPicture picture = inputs[i];
                all[3 + 2 * i] = picture != null ? picture.width : 1;
                all[4 + 2 * i] = picture != null ? picture.height : 1;
            }
        }
        float w = Math.max(0, getWidth() - 2 * margin) / density;
        float h = Math.max(0, getHeight() - 2 * margin) / density;
        all[0] = w;
        all[1] = h;
        all[2] = density;
        Bitmap[] bitmaps = new Bitmap[images.length];
        List<CanvasImages<CanvasNode.Picture>.Lease> leases = new ArrayList<>();
        for (int i = 0; i < images.length; i++) {
            if (pixels[i] != null) {
                if (i < p.images) {
                    all[3 + 2 * i] = pixels[i].width;
                    all[4 + 2 * i] = pixels[i].height;
                }
                continue;
            }
            if (images[i] == null) continue;
            CanvasImages<CanvasNode.Picture>.Lease lease = pictures.acquire(images[i]);
            if (lease == null) continue;
            leases.add(lease);
            CanvasNode.Picture pic = lease.value;
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
            framePixels = pixels;
            frameInputs = inputs;
            frameRequired = requiredPictures;
            frameGeneration = generation;
            releaseLeases(frameLeases);
            frameLeases = leases;
            frameBitmaps = bitmaps;
            width = getWidth();
            height = getHeight();
            if (pending) return;
            pending = true;
        }
        start();
        gl.post(this::render);
    }

    /** Forgets the programs Go released, which no view draws any more, on the UI thread; the GL thread deletes them. */
    static void release(int[] ids) {
        List<Program> gone = new ArrayList<>();
        for (int id : ids) {
            Program p = programs.remove(id);
            if (p != null) gone.add(p);
        }
        // Without the GL thread, none was linked.
        if (gone.isEmpty() || gl == null) return;
        gl.post(() -> {
            if (failed) return;
            released.addAll(gone);
            // Without a surface, in the context alone where EGL allows it; else at the next draw.
            if (context.equals(EGL14.eglGetCurrentContext())
                    || EGL14.eglMakeCurrent(display, EGL14.EGL_NO_SURFACE, EGL14.EGL_NO_SURFACE, context)) {
                deleteReleased();
            }
        });
    }

    /** Deletes the released programs, on the GL thread with the shared context current; none links again. */
    private static void deleteReleased() {
        for (Program p : released) {
            if (p.id > 0) {
                GLES30.glDeleteProgram(p.id);
                GLES30.glDeleteBuffers(1, new int[] {p.ubo}, 0);
                GLES30.glDeleteVertexArrays(1, new int[] {p.vao}, 0);
            }
            p.id = 0;
        }
        released.clear();
    }

    /** Starts the GL thread and its context, once. */
    private static synchronized void start() {
        if (thread != null) return;
        thread = new HandlerThread("nsgo-shaders");
        thread.start();
        gl = new Handler(thread.getLooper());
        gl.post(ShaderNode::makeContext);
    }

    static Handler threadHandler() {
        start();
        return gl;
    }

    /** Makes the shared context current for texture uploads without a view surface. */
    static boolean pictureContext() {
        if (failed) return false;
        // Uploads and blits can use a window's current context between draws.
        if (context.equals(EGL14.eglGetCurrentContext())) return true;
        if (pictureSurface == null) {
            pictureSurface = EGL14.eglCreatePbufferSurface(display, config,
                    new int[] {EGL14.EGL_WIDTH, 1, EGL14.EGL_HEIGHT, 1, EGL14.EGL_NONE}, 0);
        }
        return EGL14.eglMakeCurrent(display, pictureSurface, pictureSurface, context);
    }

    private static void makeContext() {
        display = EGL14.eglGetDisplay(EGL14.EGL_DEFAULT_DISPLAY);
        int[] version = new int[2];
        if (!EGL14.eglInitialize(display, version, 0, version, 1)) {
            failed = true;
            return;
        }
        int[] attribs = {EGL14.EGL_RENDERABLE_TYPE, 0x40 /* EGL_OPENGL_ES3_BIT */,
                EGL14.EGL_SURFACE_TYPE, EGL14.EGL_WINDOW_BIT | EGL14.EGL_PBUFFER_BIT, EGL14.EGL_RED_SIZE, 8,
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
        shownGeneration = -1;
        // The surface goes on the GL thread, then the texture with it.
        gl.post(() -> {
            EGLSurface s;
            synchronized (this) {
                s = surface;
                surface = null;
                releaseLeases(frameLeases);
                frameLeases = null;
                frameBitmaps = null;
                frameInputs = null;
                frame = null;
            }
            if (s != null) {
                // The textures go with it; images come back when it does.
                boolean pixelTexture = false;
                for (int t : pixelTextures) pixelTexture |= t != 0;
                if ((!textures.isEmpty() || pixelTexture) && EGL14.eglMakeCurrent(display, s, s, context)) {
                    int[] ids = new int[textures.size()];
                    int i = 0;
                    for (int id : textures.values()) ids[i++] = id;
                    if (ids.length > 0) GLES30.glDeleteTextures(ids.length, ids, 0);
                    for (int j = 0; j < pixelTextures.length; j++) {
                        if (pixelTextures[j] != 0) GLES30.glDeleteTextures(1, pixelTextures, j);
                    }
                }
                textures.clear();
                for (int i = 0; i < pixelTextures.length; i++) {
                    pixelTextures[i] = 0;
                    uploaded[i] = null;
                }
                EGL14.eglMakeCurrent(display, EGL14.EGL_NO_SURFACE, EGL14.EGL_NO_SURFACE, EGL14.EGL_NO_CONTEXT);
                EGL14.eglDestroySurface(display, s);
            }
            texture.release();
        });
        return false;
    }

    @Override
    public void onSurfaceTextureUpdated(SurfaceTexture texture) {}

    private static void releaseLeases(List<CanvasImages<CanvasNode.Picture>.Lease> leases) {
        if (leases != null) for (CanvasImages<CanvasNode.Picture>.Lease lease : leases) lease.close();
    }

    /** Draws the latest frame, on the GL thread. */
    private void render() {
        EGLSurface s;
        ByteBuffer block;
        String[] imgs;
        Pixels[] px;
        ShaderPicture[] inputs;
        int required, generation;
        Bitmap[] bitmaps;
        List<CanvasImages<CanvasNode.Picture>.Lease> leases;
        int w, h;
        Program p;
        synchronized (this) {
            pending = false;
            s = surface;
            block = frame;
            p = frameProgram;
            imgs = frameImages;
            px = framePixels;
            inputs = frameInputs;
            required = frameRequired;
            generation = frameGeneration;
            bitmaps = frameBitmaps;
            leases = frameLeases;
            frameLeases = null;
            frameBitmaps = null;
            frame = null;
            w = width;
            h = height;
        }
        try {
            if (failed || s == null || block == null || p == null || w <= 0 || h <= 0) return;
            if (inputs != null) {
                for (int i = 0; i < inputs.length; i++) {
                    if ((required & (1 << i)) != 0 && (inputs[i] == null || inputs[i].texture == 0)) return;
                }
            }
            if (!EGL14.eglMakeCurrent(display, s, s, context)) return;
            deleteReleased();
            if (p.id == -1 && !link(p)) p.id = 0;
            GLES30.glViewport(0, 0, w, h);
            GLES30.glClearColor(0, 0, 0, 0);
            GLES30.glClear(GLES30.GL_COLOR_BUFFER_BIT);
            if (p.id > 0) {
                GLES30.glUseProgram(p.id);
                GLES30.glUniform1f(p.height, h);
                for (int i = 0; i < p.images; i++) {
                    GLES30.glActiveTexture(GLES30.GL_TEXTURE0 + i);
                    int t = 0;
                    if (inputs != null) {
                        ShaderPicture pic = inputs[i];
                        t = pic != null && pic.texture != 0 ? pic.texture : emptyPicture();
                    } else if (i < px.length && px[i] != null) {
                        t = pixelTexture(i, px[i]);
                    } else {
                        if (i < pixelTextures.length && pixelTextures[i] != 0) {
                            // The image is no longer pixels.
                            GLES30.glDeleteTextures(1, pixelTextures, i);
                            pixelTextures[i] = 0;
                            uploaded[i] = null;
                        }
                        if (i < imgs.length && imgs[i] != null) t = texture(imgs[i], bitmaps[i]);
                    }
                    GLES30.glBindTexture(GLES30.GL_TEXTURE_2D, t);
                }
                // A program reading fewer images leaves the others' textures unused.
                for (int i = p.images; i < pixelTextures.length; i++) {
                    if (pixelTextures[i] == 0) continue;
                    GLES30.glDeleteTextures(1, pixelTextures, i);
                    pixelTextures[i] = 0;
                    uploaded[i] = null;
                }
                GLES30.glBindBuffer(GLES30.GL_UNIFORM_BUFFER, p.ubo);
                block.position(0);
                GLES30.glBufferSubData(GLES30.GL_UNIFORM_BUFFER, 0, block.capacity(), block);
                GLES30.glBindBufferBase(GLES30.GL_UNIFORM_BUFFER, 0, p.ubo);
                GLES30.glBindVertexArray(p.vao);
                GLES30.glDrawArrays(GLES30.GL_TRIANGLES, 0, 3);
            }
            if (EGL14.eglSwapBuffers(display, s) && inputs != null && p.id > 0 && shownGeneration != generation) {
                shownGeneration = generation;
                post(() -> {
                    if (effectReady() && pictureShown != null) pictureShown.run();
                });
            }
        } finally {
            releaseLeases(leases);
        }
    }

    /**
     * Returns the texture of image i, whose pixels are px: written into the
     * texture the image had when its size is the same, and not at all when it
     * holds px already.
     */
    private int pixelTexture(int i, Pixels px) {
        if (i >= pixelTextures.length) return 0;
        Pixels had = uploaded[i];
        if (had == px && pixelTextures[i] != 0) return pixelTextures[i];
        px.bytes.position(0);
        if (pixelTextures[i] != 0 && had != null && had.width == px.width && had.height == px.height) {
            GLES30.glBindTexture(GLES30.GL_TEXTURE_2D, pixelTextures[i]);
            GLES30.glTexSubImage2D(GLES30.GL_TEXTURE_2D, 0, 0, 0, px.width, px.height, GLES30.GL_RGBA,
                    GLES30.GL_UNSIGNED_BYTE, px.bytes);
        } else {
            if (pixelTextures[i] == 0) GLES30.glGenTextures(1, pixelTextures, i);
            GLES30.glBindTexture(GLES30.GL_TEXTURE_2D, pixelTextures[i]);
            GLES30.glTexImage2D(GLES30.GL_TEXTURE_2D, 0, GLES30.GL_RGBA, px.width, px.height, 0, GLES30.GL_RGBA,
                    GLES30.GL_UNSIGNED_BYTE, px.bytes);
            GLES30.glTexParameteri(GLES30.GL_TEXTURE_2D, GLES30.GL_TEXTURE_MIN_FILTER, GLES30.GL_NEAREST);
            GLES30.glTexParameteri(GLES30.GL_TEXTURE_2D, GLES30.GL_TEXTURE_MAG_FILTER, GLES30.GL_NEAREST);
            GLES30.glTexParameteri(GLES30.GL_TEXTURE_2D, GLES30.GL_TEXTURE_WRAP_S, GLES30.GL_CLAMP_TO_EDGE);
            GLES30.glTexParameteri(GLES30.GL_TEXTURE_2D, GLES30.GL_TEXTURE_WRAP_T, GLES30.GL_CLAMP_TO_EDGE);
        }
        uploaded[i] = px;
        return pixelTextures[i];
    }

    /** The transparent picture a role does not supply. The shader thread owns it. */
    private static int emptyPicture() {
        if (emptyPicture != 0) return emptyPicture;
        int[] names = new int[1];
        GLES30.glGenTextures(1, names, 0);
        emptyPicture = names[0];
        GLES30.glBindTexture(GLES30.GL_TEXTURE_2D, emptyPicture);
        ByteBuffer pixel = ByteBuffer.allocateDirect(4);
        GLES30.glTexImage2D(GLES30.GL_TEXTURE_2D, 0, GLES30.GL_RGBA, 1, 1, 0,
                GLES30.GL_RGBA, GLES30.GL_UNSIGNED_BYTE, pixel);
        GLES30.glTexParameteri(GLES30.GL_TEXTURE_2D, GLES30.GL_TEXTURE_MIN_FILTER, GLES30.GL_NEAREST);
        GLES30.glTexParameteri(GLES30.GL_TEXTURE_2D, GLES30.GL_TEXTURE_MAG_FILTER, GLES30.GL_NEAREST);
        return emptyPicture;
    }

    /** Returns image key's texture, made from bitmap once it decoded; 0 until then. */
    private int texture(String key, Bitmap bitmap) {
        Integer t = textures.get(key);
        if (t != null) return t;
        if (bitmap == null) return 0;
        int unkept = 0;
        for (String k : textures.keySet()) {
            if (!pictures.isKept(k)) unkept++;
        }
        int reserve = pictures.isKept(key) ? 0 : 1;
        while (unkept + reserve > CanvasImages.LIMIT) {
            String oldest = null;
            for (String k : textures.keySet()) {
                if (!pictures.isKept(k)) {
                    oldest = k;
                    break;
                }
            }
            if (oldest == null) break;
            GLES30.glDeleteTextures(1, new int[] {textures.remove(oldest)}, 0);
            unkept--;
        }
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

    /** Links a full-frame fragment program in the shared context. */
    static int linkProgram(String source) {
        int vs = compile(GLES30.GL_VERTEX_SHADER, VERTEX);
        if (vs == 0) return 0;
        int fs = compile(GLES30.GL_FRAGMENT_SHADER, source);
        if (fs == 0) {
            GLES30.glDeleteShader(vs);
            return 0;
        }
        int prog = GLES30.glCreateProgram();
        GLES30.glAttachShader(prog, vs);
        GLES30.glAttachShader(prog, fs);
        GLES30.glLinkProgram(prog);
        GLES30.glDeleteShader(vs);
        GLES30.glDeleteShader(fs);
        int[] ok = new int[1];
        GLES30.glGetProgramiv(prog, GLES30.GL_LINK_STATUS, ok, 0);
        if (ok[0] != 0) return prog;
        Log.w(TAG, "shader: GLES: " + GLES30.glGetProgramInfoLog(prog));
        GLES30.glDeleteProgram(prog);
        return 0;
    }

    /** Links p and binds its images and uniform block. */
    private static boolean link(Program p) {
        int prog = linkProgram(p.source);
        if (prog == 0) return false;
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
