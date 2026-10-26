package dev.oneo.nsgo;

import android.view.View;

/**
 * A view of the embedding app's own that an NSGo app shows with a Native
 * element of the name it was registered under
 * (NsgoView.registerComponent). Its view fills the element's box and
 * measures its natural size where the app's style leaves one open. Props
 * and commands come as org.json's values made plain: maps, lists,
 * strings, numbers, booleans and null. Its methods run on the UI thread.
 */
public interface NsgoComponent {
    /** The view that fills the element's box. */
    View view();

    /** Sets one of the element's props; null when the prop goes. */
    void setProp(String name, Object value);

    /** Sets an animated prop to its value on a frame it changed; setProp by default. */
    default void setAnimatedNumber(String name, double value) {
        setProp(name, value);
    }

    /** Sets an animated color prop, as an ARGB int, on a frame it changed; setProp by default. */
    default void setAnimatedColor(String name, int argb) {
        setProp(name, argb);
    }

    /** Runs a command the app sent (core.Ref.Command). */
    default void command(String name, Object args) {}

    /** Releases what the component holds, once its element or its app is gone. */
    default void destroy() {}

    /** Makes a component for each element, with how it talks back. */
    interface Factory {
        NsgoComponent create(android.content.Context context, Events events);
    }

    /** How a component talks back to its element: what it emits reaches the element's OnEvent. */
    interface Events {
        void emit(String name, Object data);
    }
}
