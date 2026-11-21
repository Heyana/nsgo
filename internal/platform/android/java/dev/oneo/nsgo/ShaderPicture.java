package dev.oneo.nsgo;

import android.annotation.TargetApi;
import android.graphics.Canvas;
import android.graphics.HardwareRenderer;
import android.graphics.RecordingCanvas;
import android.graphics.RenderNode;
import android.graphics.SurfaceTexture;
import android.opengl.GLES30;
import android.os.Handler;
import android.os.Looper;
import android.util.Log;
import android.view.Surface;
import java.util.function.Consumer;

/** A native display list rendered into a shader texture without reading pixels back to the CPU. */
@TargetApi(29)
final class ShaderPicture {
    private static final Handler ui = new Handler(Looper.getMainLooper());
    private static final String COPY = "#version 300 es\n"
            + "#extension GL_OES_EGL_image_external_essl3 : require\n"
            + "precision highp float;\n"
            + "uniform samplerExternalOES picture;\n"
            + "uniform mat4 transform;\n"
            + "uniform vec2 size;\n"
            + "out vec4 color;\n"
            + "void main() {\n"
            + "    vec2 uv = gl_FragCoord.xy / size;\n"
            + "    vec2 tex = (transform * vec4(uv.x, 1.0 - uv.y, 0.0, 1.0)).xy;\n"
            + "    color = texture(picture, tex);\n"
            + "}\n";

    // The shader thread owns the copy program and each picture's textures.
    private static int copyProgram = -1, transformUniform, sizeUniform, vao;
    private int framebuffer;
    private boolean attached;
    private final float[] transform = new float[16];
    int texture;

    final int width, height;
    private final RenderNode node = new RenderNode("nsgo-shader-picture");
    private final HardwareRenderer renderer;
    private final SurfaceTexture buffer;
    private final Surface surface;
    private final Runnable updated;
    private boolean pending;
    private volatile boolean closed;

    ShaderPicture(int width, int height, Runnable updated) {
        this.width = width;
        this.height = height;
        this.updated = updated;
        buffer = new SurfaceTexture(false);
        buffer.setDefaultBufferSize(width, height);
        surface = new Surface(buffer);
        HardwareRenderer next = null;
        try {
            next = new HardwareRenderer();
            next.setOpaque(false);
            next.setSurface(surface);
            next.setContentRoot(node);
            node.setPosition(0, 0, width, height);
            Handler gl = ShaderNode.threadHandler();
            gl.post(this::attach);
            buffer.setOnFrameAvailableListener(this::copy, gl);
        } catch (RuntimeException e) {
            if (next != null) next.destroy();
            surface.release();
            buffer.release();
            throw e;
        }
        renderer = next;
    }

    /** Records at most one outstanding capture. The caller can retry when updated runs. */
    boolean take(Consumer<Canvas> draw) {
        if (pending || closed) return false;
        RecordingCanvas canvas = node.beginRecording(width, height);
        try {
            draw.accept(canvas);
        } finally {
            node.endRecording();
        }
        int result = renderer.createRenderRequest().setWaitForPresent(false).syncAndDraw();
        pending = (result & (HardwareRenderer.SYNC_LOST_SURFACE_REWARD_IF_FOUND
                | HardwareRenderer.SYNC_CONTEXT_IS_STOPPED | HardwareRenderer.SYNC_FRAME_DROPPED)) == 0;
        return pending;
    }

    private void attach() {
        if (closed || !ShaderNode.pictureContext()) return;
        int[] names = new int[1];
        GLES30.glGenTextures(1, names, 0);
        try {
            buffer.attachToGLContext(names[0]);
            attached = true;
        } catch (RuntimeException e) {
            GLES30.glDeleteTextures(1, names, 0);
            Log.w("nsgo", "shader picture surface", e);
        }
    }

    private void copy(SurfaceTexture source) {
        if (closed) return;
        try {
            if (attached && ShaderNode.pictureContext()) {
                GLES30.glActiveTexture(GLES30.GL_TEXTURE0);
                source.updateTexImage();
                if (prepareCopy()) {
                    source.getTransformMatrix(transform);
                    GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, framebuffer);
                    GLES30.glViewport(0, 0, width, height);
                    GLES30.glUseProgram(copyProgram);
                    GLES30.glUniformMatrix4fv(transformUniform, 1, false, transform, 0);
                    GLES30.glUniform2f(sizeUniform, width, height);
                    GLES30.glBindVertexArray(vao);
                    GLES30.glDrawArrays(GLES30.GL_TRIANGLES, 0, 3);
                    GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, 0);
                }
            }
        } catch (RuntimeException e) {
            Log.w("nsgo", "shader picture texture", e);
        }
        ui.post(() -> {
            if (closed) return;
            pending = false;
            updated.run();
        });
    }

    private boolean prepareCopy() {
        if (copyProgram == -1) linkCopy();
        if (copyProgram == 0) return false;
        if (texture != 0) return true;
        int[] names = new int[2];
        GLES30.glGenTextures(1, names, 0);
        GLES30.glBindTexture(GLES30.GL_TEXTURE_2D, names[0]);
        GLES30.glTexStorage2D(GLES30.GL_TEXTURE_2D, 1, GLES30.GL_RGBA8, width, height);
        GLES30.glTexParameteri(GLES30.GL_TEXTURE_2D, GLES30.GL_TEXTURE_MIN_FILTER, GLES30.GL_NEAREST);
        GLES30.glTexParameteri(GLES30.GL_TEXTURE_2D, GLES30.GL_TEXTURE_MAG_FILTER, GLES30.GL_NEAREST);
        GLES30.glGenFramebuffers(1, names, 1);
        GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, names[1]);
        GLES30.glFramebufferTexture2D(GLES30.GL_FRAMEBUFFER, GLES30.GL_COLOR_ATTACHMENT0, GLES30.GL_TEXTURE_2D, names[0], 0);
        boolean ready = GLES30.glCheckFramebufferStatus(GLES30.GL_FRAMEBUFFER) == GLES30.GL_FRAMEBUFFER_COMPLETE;
        GLES30.glBindFramebuffer(GLES30.GL_FRAMEBUFFER, 0);
        if (ready) {
            texture = names[0];
            framebuffer = names[1];
        } else {
            GLES30.glDeleteTextures(1, names, 0);
            GLES30.glDeleteFramebuffers(1, names, 1);
        }
        return ready;
    }

    private static void linkCopy() {
        copyProgram = ShaderNode.linkProgram(COPY);
        if (copyProgram == 0) return;
        GLES30.glUseProgram(copyProgram);
        GLES30.glUniform1i(GLES30.glGetUniformLocation(copyProgram, "picture"), 0);
        transformUniform = GLES30.glGetUniformLocation(copyProgram, "transform");
        sizeUniform = GLES30.glGetUniformLocation(copyProgram, "size");
        int[] names = new int[1];
        GLES30.glGenVertexArrays(1, names, 0);
        vao = names[0];
    }

    void close() {
        if (closed) return;
        closed = true;
        renderer.destroy();
        node.discardDisplayList();
        surface.release();
        ShaderNode.threadHandler().post(() -> {
            if (ShaderNode.pictureContext()) {
                if (attached) {
                    try {
                        buffer.detachFromGLContext();
                    } catch (RuntimeException e) {
                        Log.w("nsgo", "releasing shader picture surface", e);
                    }
                }
                if (texture != 0) {
                    GLES30.glDeleteTextures(1, new int[] {texture}, 0);
                    GLES30.glDeleteFramebuffers(1, new int[] {framebuffer}, 0);
                    texture = 0;
                }
            }
            buffer.release();
        });
    }
}
