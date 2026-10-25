package dev.oneo.nsgo;

import android.content.Context;
import android.util.Log;
import android.view.View;
import android.view.ViewGroup;
import java.lang.reflect.Method;
import java.lang.reflect.Modifier;
import java.lang.reflect.Proxy;
import java.util.HashSet;
import java.util.Locale;
import java.util.Set;
import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;
import org.json.JSONTokener;

/**
 * A native component's view. This departs from NSH, whose components are
 * Java objects the embedding app registers: the Go process cannot make
 * views, so a component names a View class and the runtime drives it by
 * reflection. It builds the class with its (Context) constructor, sets a
 * prop by calling the public one-argument set method of its name (prop
 * "rating" calls setRating) with the JSON value coerced to the parameter,
 * runs a command by calling the public method of its name with the
 * arguments, and reports each callback of the listeners its class
 * declares (its own setOn...Listener methods, not View's or ViewGroup's)
 * as an event named after the callback with its arguments.
 */
final class Embedded {
    private static final String TAG = "nsgo";
    /** What coerce returns for a value the parameter cannot take. */
    private static final Object NO = new Object();
    /** Classes already logged as unusable. */
    private static final Set<String> logged = new HashSet<>();

    /** Receives a listener callback: its name and arguments as a JSON array. */
    interface Emitter {
        void emit(String name, String args);
    }

    final View view;
    /** The size last reported, in pixels. */
    int width = -1, height = -1;

    /** The embedding app's component, for one it registered; null for a class built by reflection. */
    private final NsgoComponent hosted;

    private Embedded(View view, NsgoComponent hosted) {
        this.view = view;
        this.hosted = hosted;
    }

    /** Wraps a component the embedding app registered (NsgoView.registerComponent). */
    static Embedded hosted(NsgoComponent c) {
        return new Embedded(c.view(), c);
    }

    /** Destroys the embedding app's component; a view built by reflection only goes. */
    void destroy() {
        if (hosted != null) hosted.destroy();
    }

    /** Builds a view of class name, or returns null for a class that is no View, logged once. */
    static Embedded build(Context context, String name, Emitter emit) {
        View view;
        try {
            Class<?> c = Class.forName(name, true, context.getClassLoader());
            if (!View.class.isAssignableFrom(c)) throw new ClassCastException(name + " is no android.view.View");
            view = (View) c.getConstructor(Context.class).newInstance(context);
        } catch (Throwable e) {
            if (logged.add(name)) Log.w(TAG, "native component view " + name + " cannot be built: it shows an empty box", e);
            return null;
        }
        listen(view, emit);
        return new Embedded(view, null);
    }

    /** Has each listener the view's own class declares report its callbacks. */
    private static void listen(View view, Emitter emit) {
        for (Class<?> c = view.getClass(); c != null && c != View.class && c != ViewGroup.class; c = c.getSuperclass()) {
            for (Method m : c.getDeclaredMethods()) {
                String name = m.getName();
                Class<?>[] params = m.getParameterTypes();
                if (!Modifier.isPublic(m.getModifiers()) || !name.startsWith("setOn") || !name.endsWith("Listener")
                        || params.length != 1 || !params[0].isInterface()) {
                    continue;
                }
                Object listener = Proxy.newProxyInstance(view.getClass().getClassLoader(), new Class<?>[] {params[0]},
                        (proxy, method, args) -> {
                            if (method.getDeclaringClass() == Object.class) {
                                switch (method.getName()) {
                                    case "equals": return proxy == args[0];
                                    case "hashCode": return System.identityHashCode(proxy);
                                    default: return params[0].getName() + "@nsgo";
                                }
                            }
                            emit.emit(method.getName(), json(args));
                            return zero(method.getReturnType());
                        });
                try {
                    m.invoke(view, listener);
                } catch (Throwable e) {
                    // Some refuse a listener, as Spinner's item clicks.
                }
            }
        }
    }

    /** Calls set<Name> with value, a parsed JSON value. */
    void setProp(String name, Object value) {
        if (hosted != null) {
            hosted.setProp(name, Host.plain(value));
            return;
        }
        if (name.isEmpty()) return;
        String setter = "set" + name.substring(0, 1).toUpperCase(Locale.ROOT) + name.substring(1);
        if (!call(setter, new Object[] {value})) log("no public method " + setter + " takes " + value);
    }

    /** Calls method name with args: a JSON array's elements, nothing for null, else the one value. */
    void command(String name, Object args) {
        if (hosted != null) {
            hosted.command(name, Host.plain(args));
            return;
        }
        Object[] list;
        if (args instanceof JSONArray) {
            JSONArray a = (JSONArray) args;
            list = new Object[a.length()];
            for (int i = 0; i < list.length; i++) list[i] = a.opt(i);
        } else if (args == null || args == JSONObject.NULL) {
            list = new Object[0];
        } else {
            list = new Object[] {args};
        }
        if (!call(name, list)) log("no public method " + name + " takes " + list.length + " such arguments");
    }

    private void log(String message) {
        if (logged.add(view.getClass().getName() + "." + message)) Log.w(TAG, view.getClass().getName() + ": " + message);
    }

    /** Invokes the first public method of name whose parameters take args. */
    private boolean call(String name, Object[] args) {
        for (Method m : view.getClass().getMethods()) {
            Class<?>[] params = m.getParameterTypes();
            if (!m.getName().equals(name) || params.length != args.length) continue;
            Object[] values = new Object[args.length];
            boolean ok = true;
            for (int i = 0; i < args.length && ok; i++) {
                values[i] = coerce(args[i], params[i]);
                ok = values[i] != NO;
            }
            if (!ok) continue;
            try {
                m.invoke(view, values);
            } catch (Throwable e) {
                Log.w(TAG, view.getClass().getName() + "." + name + " failed", e);
            }
            return true;
        }
        return false;
    }

    /** A JSON value as a parameter of type t, or NO. */
    private static Object coerce(Object v, Class<?> t) {
        if (v == null || v == JSONObject.NULL) return t.isPrimitive() ? NO : null;
        if (t == boolean.class || t == Boolean.class) return v instanceof Boolean ? v : NO;
        if (v instanceof Number) {
            Number n = (Number) v;
            if (t == int.class || t == Integer.class) return (int) Math.round(n.doubleValue());
            if (t == long.class || t == Long.class) return Math.round(n.doubleValue());
            if (t == float.class || t == Float.class) return n.floatValue();
            if (t == double.class || t == Double.class) return n.doubleValue();
            if (t == short.class || t == Short.class) return (short) Math.round(n.doubleValue());
            if (t == byte.class || t == Byte.class) return (byte) Math.round(n.doubleValue());
        }
        if (v instanceof String && t.isAssignableFrom(String.class)) return v;
        if (v instanceof Boolean && t.isAssignableFrom(Boolean.class)) return v;
        if (v instanceof Number && t.isAssignableFrom(Double.class)) return ((Number) v).doubleValue();
        return NO;
    }

    /** The value a listener callback returns: false, 0 or null, so the view goes on as without it. */
    private static Object zero(Class<?> t) {
        if (t == boolean.class) return false;
        if (t == int.class) return 0;
        if (t == long.class) return 0L;
        if (t == float.class) return 0f;
        if (t == double.class) return 0d;
        if (t == short.class) return (short) 0;
        if (t == byte.class) return (byte) 0;
        if (t == char.class) return (char) 0;
        return null;
    }

    /** Callback arguments as a JSON array: booleans, finite numbers and text, null for the rest. */
    private static String json(Object[] args) {
        JSONArray a = new JSONArray();
        if (args != null) {
            for (Object x : args) {
                Object v = JSONObject.NULL;
                if (x instanceof Boolean || x instanceof String) {
                    v = x;
                } else if (x instanceof CharSequence) {
                    v = x.toString();
                } else if (x instanceof Number) {
                    double d = ((Number) x).doubleValue();
                    if (!Double.isNaN(d) && !Double.isInfinite(d)) v = x instanceof Float ? (Object) d : x;
                }
                a.put(v);
            }
        }
        return a.toString();
    }

    /** Parses a JSON value: null for one that is no JSON. */
    static Object parse(String json) {
        try {
            return new JSONTokener(json).nextValue();
        } catch (JSONException e) {
            return null;
        }
    }

    /** The view's natural size in pixels, measured without constraints. */
    int[] measure() {
        int spec = View.MeasureSpec.makeMeasureSpec(0, View.MeasureSpec.UNSPECIFIED);
        view.measure(spec, spec);
        return new int[] {view.getMeasuredWidth(), view.getMeasuredHeight()};
    }
}
