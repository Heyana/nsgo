package dev.oneo.nsgo;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.Context;
import android.content.ContextWrapper;
import android.content.Intent;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageManager;
import android.content.pm.ResolveInfo;
import android.view.ActionMode;
import android.view.Menu;
import android.view.MenuItem;
import android.view.ViewGroup;
import java.io.BufferedOutputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Locale;

/** Exercises Selection's ACTION_PROCESS_TEXT query, filtering, floating toolbar population and launch. */
public final class ProcessTextCheck {
    private static final class FakeItem implements MenuItem {
        final int group, id, order;
        final String title;
        int showAsAction;

        FakeItem(int group, int id, int order, String title) {
            this.group = group;
            this.id = id;
            this.order = order;
            this.title = title;
        }

        @Override public int getItemId() { return id; }
        @Override public int getGroupId() { return group; }
        @Override public int getOrder() { return order; }
        @Override public void setShowAsAction(int actionEnum) { showAsAction = actionEnum; }
    }

    private static final class FakeMenu implements Menu {
        final List<FakeItem> items = new ArrayList<>();

        @Override
        public MenuItem add(int group, int id, int order, int title) {
            FakeItem item = new FakeItem(group, id, order, "res:" + title);
            items.add(item);
            return item;
        }

        @Override
        public MenuItem add(int group, int id, int order, CharSequence title) {
            FakeItem item = new FakeItem(group, id, order, title == null ? "" : title.toString());
            items.add(item);
            return item;
        }

        @Override
        public void removeGroup(int group) {
            items.removeIf(item -> item.group == group);
        }
    }

    private static final class FakeActionMode extends ActionMode {
        final Callback2 callback;
        final FakeMenu menu = new FakeMenu();
        boolean finished;
        int invalidates;

        FakeActionMode(Callback2 callback) {
            this.callback = callback;
        }

        @Override
        public void finish() {
            finished = true;
            callback.onDestroyActionMode(this);
        }

        @Override
        public void invalidate() {
            invalidates++;
            callback.onPrepareActionMode(this, menu);
        }

        @Override
        public void invalidateContentRect() {}
    }

    private static final class ToolbarHost extends ViewGroup {
        int lastModeType = -1;
        FakeActionMode mode;

        ToolbarHost(Context context) {
            super(context);
        }

        @Override
        public ActionMode startActionMode(ActionMode.Callback2 callback, int type) {
            lastModeType = type;
            FakeActionMode m = new FakeActionMode(callback);
            callback.onCreateActionMode(m, m.menu);
            callback.onPrepareActionMode(m, m.menu);
            mode = m;
            return m;
        }
    }

    private static void writeIntent(DataOutputStream out, Intent intent) throws IOException {
        Protocol.writeString(out, intent.getAction());
        Protocol.writeString(out, intent.getType());
        Protocol.writeString(out, intent.getPackageName());
        Protocol.writeString(out, intent.getClassName());
        Protocol.writeString(out, intent.getStringExtra(Intent.EXTRA_PROCESS_TEXT));
        out.writeBoolean(intent.getBooleanExtra(Intent.EXTRA_PROCESS_TEXT_READONLY, false));
        out.writeInt(intent.getFlags());
    }

    public static void main(String[] args) throws IOException {
        Locale.setDefault(Locale.ROOT);
        Context base = new Context();
        base.grantedPermissions.add("com.example.PERM_GRANTED");
        PackageManager pm = new PackageManager(Arrays.asList(
                new ResolveInfo(new ActivityInfo("com.google.android.apps.translate", "com.google.android.apps.translate.TranslateActivity", true, null), "Translate"),
                new ResolveInfo(new ActivityInfo("com.example.private", "com.example.private.HiddenActivity", false, null), "Hidden"),
                new ResolveInfo(new ActivityInfo("com.example.denied", "com.example.denied.DeniedActivity", true, "com.example.PERM_DENIED"), "Denied"),
                new ResolveInfo(new ActivityInfo("com.example.allowed", "com.example.allowed.AllowedActivity", true, "com.example.PERM_GRANTED"), "Dictionary"),
                new ResolveInfo(new ActivityInfo("com.example.empty", "com.example.empty.EmptyLabelActivity", true, null), ""),
                new ResolveInfo(new ActivityInfo("dev.oneo.nsgo.test", "dev.oneo.nsgo.test.InternalActivity", false, null), "Own Action"),
                new ResolveInfo(null, "Broken")
        ));
        base.packageManager = pm;

        // 1. Activity-wrapped context (like ContextThemeWrapper around NsgoActivity): should NOT add FLAG_ACTIVITY_NEW_TASK.
        Context wrappedActivity = new ContextWrapper(new Activity(base));
        ToolbarHost host = new ToolbarHost(wrappedActivity);
        ViewGroup area = new ViewGroup(wrappedActivity);
        area.addView(new TextNode(wrappedActivity, "alpha beta", 0, 0, 100, 20));
        Selection selection = new Selection(host, area);
        selection.selectWordAt(15, 5); // "alpha"
        selection.showToolbar();

        // Tap Translate on "alpha".
        FakeItem translateItem = host.mode.menu.items.get(2);
        boolean handledTranslate = host.mode.callback.onActionItemClicked(host.mode, translateItem);
        boolean finishedAfterTranslate = host.mode.finished;

        // Show toolbar again, tap Select All, then tap Dictionary to confirm current text is launched.
        selection.showToolbar();
        FakeItem selectAllItem = host.mode.menu.items.get(1);
        host.mode.callback.onActionItemClicked(host.mode, selectAllItem);
        FakeItem dictionaryItem = host.mode.menu.items.get(3);
        boolean handledDictionary = host.mode.callback.onActionItemClicked(host.mode, dictionaryItem);

        // 2. Non-Activity context (like an overlay Service context): MUST add FLAG_ACTIVITY_NEW_TASK.
        ToolbarHost overlayHost = new ToolbarHost(base);
        ViewGroup overlayArea = new ViewGroup(base);
        overlayArea.addView(new TextNode(base, "gamma delta", 0, 0, 110, 20));
        Selection overlaySel = new Selection(overlayHost, overlayArea);
        overlaySel.selectWordAt(15, 5); // "gamma"
        overlaySel.showToolbar();
        FakeItem ownItem = overlayHost.mode.menu.items.get(4);
        boolean handledOwn = overlayHost.mode.callback.onActionItemClicked(overlayHost.mode, ownItem);

        // 3. Launching when the target activity is missing or refused returns false without throwing.
        base.startFailure = new ActivityNotFoundException("gone");
        boolean missingOk = Selection.launchProcessText(base, new ActivityInfo("com.missing", "com.missing.A", true, null), "gamma");
        base.startFailure = new SecurityException("denied");
        boolean deniedOk = Selection.launchProcessText(base, new ActivityInfo("com.denied", "com.denied.A", true, null), "gamma");
        base.startFailure = null;

        try (DataOutputStream out = new DataOutputStream(new BufferedOutputStream(System.out))) {
            // Query intent checks.
            Protocol.writeString(out, pm.lastQuery.getAction());
            Protocol.writeString(out, pm.lastQuery.getType());
            out.writeInt(pm.lastFlags);

            // Floating toolbar mode and menu items.
            out.writeInt(host.lastModeType);
            out.writeInt(host.mode.menu.items.size());
            for (FakeItem item : host.mode.menu.items) {
                out.writeInt(item.group);
                out.writeInt(item.id);
                out.writeInt(item.order);
                out.writeInt(item.showAsAction);
                Protocol.writeString(out, item.title);
            }

            // Outcomes and launched intents.
            out.writeBoolean(handledTranslate);
            out.writeBoolean(finishedAfterTranslate);
            out.writeBoolean(handledDictionary);
            out.writeBoolean(handledOwn);
            out.writeBoolean(missingOk);
            out.writeBoolean(deniedOk);
            out.writeInt(base.started.size());
            for (Intent launched : base.started) {
                writeIntent(out, launched);
            }
        }
    }
}
