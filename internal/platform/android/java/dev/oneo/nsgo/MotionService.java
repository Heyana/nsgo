package dev.oneo.nsgo;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import java.util.ArrayList;
import java.util.LinkedHashMap;

/** Explicit, non-sticky short service. It never extends its three-minute lease
 * and never restarts after the process ends. Individual streams may end sooner. */
public final class MotionService extends Service {
    private static final LinkedHashMap<Object,Entry> entries=new LinkedHashMap<>();
    private static MotionService current;
    private final Handler ui=new Handler(Looper.getMainLooper());
    private final Runnable timeout=()->expire();
    private static final class Entry {
        final String title,body;final Runnable failed,expired;
        Entry(String title,String body,Runnable failed,Runnable expired){this.title=title;this.body=body;this.failed=failed;this.expired=expired;}
    }
    static void add(Context context,Object token,String title,String body,Runnable failed,Runnable expired){
        entries.put(token,new Entry(title,body,failed,expired));
        try{context.startForegroundService(new Intent(context,MotionService.class));}
        catch(RuntimeException e){entries.remove(token);throw e;}
    }
    static void remove(Context context,Object token){entries.remove(token);if(entries.isEmpty())context.stopService(new Intent(context,MotionService.class));else if(current!=null)current.show();}
    @Override public int onStartCommand(Intent intent,int flags,int startId){
        if(entries.isEmpty()){stopSelf();return START_NOT_STICKY;}
        if(current==null){current=this;ui.postDelayed(timeout,180000);}
        show();return START_NOT_STICKY;
    }
    private void show(){
        if(entries.isEmpty())return;
        Entry e=entries.values().iterator().next();
        getSystemService(NotificationManager.class).createNotificationChannel(new NotificationChannel("nsgo.motion",e.title,NotificationManager.IMPORTANCE_LOW));
        int icon=getApplicationInfo().icon;
        Notification.Builder b=new Notification.Builder(this,"nsgo.motion").setSmallIcon(icon==0?android.R.drawable.ic_menu_compass:icon).setContentTitle(e.title).setContentText(e.body).setOngoing(true);
        Intent launch=getPackageManager().getLaunchIntentForPackage(getPackageName());
        if(launch!=null)b.setContentIntent(PendingIntent.getActivity(this,0,launch,PendingIntent.FLAG_IMMUTABLE|PendingIntent.FLAG_UPDATE_CURRENT));
        try{if(Build.VERSION.SDK_INT>=34)startForeground(0x6e6d6f,b.build(),0x800);else startForeground(0x6e6d6f,b.build());}
        catch(RuntimeException failure){for(Entry item:new ArrayList<>(entries.values()))item.failed.run();stopSelf();}
    }
    private void expire(){for(Entry e:new ArrayList<>(entries.values()))e.expired.run();stopSelf();}
    // Android 14's shortService timeout and Android 15's typed callback.
    @Override public void onTimeout(int startId){expire();}
    @Override public void onTimeout(int startId,int type){expire();}
    @Override public void onDestroy(){
        current=null;ui.removeCallbacks(timeout);
        for(Entry e:new ArrayList<>(entries.values()))e.failed.run();entries.clear();
        stopForeground(STOP_FOREGROUND_REMOVE);super.onDestroy();
    }
    @Override public IBinder onBind(Intent intent){return null;}
}
