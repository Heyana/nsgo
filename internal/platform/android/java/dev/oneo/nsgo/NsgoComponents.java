package dev.oneo.nsgo;

import android.app.AppComponentFactory;
import android.content.BroadcastReceiver;
import android.content.Intent;

/**
 * Instantiates the stable per-kind widget components packaging declares, using
 * one prebuilt provider implementation. A kind's name is independent of its
 * position in nsgo.yaml, and app builds need neither Java nor DEX generation.
 * Android's minimum API 28 supports AppComponentFactory on every NSGo target.
 */
public final class NsgoComponents extends AppComponentFactory {
    static final String WIDGET_PREFIX = "dev.oneo.nsgo.widgets.K";

    @Override
    public BroadcastReceiver instantiateReceiver(ClassLoader loader, String name, Intent intent)
            throws InstantiationException, IllegalAccessException, ClassNotFoundException {
        if (name.startsWith(WIDGET_PREFIX)) return new AppWidgets();
        return super.instantiateReceiver(loader, name, intent);
    }
}
