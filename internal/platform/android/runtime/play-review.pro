# Retain the SDK entry points Services.java reaches by reflection and the
# Activity Android instantiates from the manifest. Keep names for Binder's
# callbacks; shrink away unrelated Google Services UI and AndroidX resources.
-dontobfuscate
-dontoptimize
-keep class com.google.android.play.core.review.** { *; }
# Keep only the framework Activity overload: AndroidX activity-result hosts
# supply their own UI and are not part of NSGo's framework-only runtime.
-keep class com.google.android.play.core.appupdate.AppUpdateManagerFactory {
    public static *** create(android.content.Context);
}
-keep interface com.google.android.play.core.appupdate.AppUpdateManager {
    public *** getAppUpdateInfo();
    public boolean startUpdateFlowForResult(com.google.android.play.core.appupdate.AppUpdateInfo, android.app.Activity, com.google.android.play.core.appupdate.AppUpdateOptions, int);
}
-keep class com.google.android.play.core.appupdate.AppUpdateInfo {
    public int updateAvailability();
    public boolean isUpdateTypeAllowed(int);
}
-keep class com.google.android.play.core.appupdate.AppUpdateOptions {
    public static *** newBuilder(int);
}
-keep class com.google.android.play.core.appupdate.AppUpdateOptions$Builder {
    public *** build();
}
-keep class com.google.android.play.core.common.PlayCoreDialogWrapperActivity { *; }
-keep class com.google.android.gms.tasks.Task { public *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
# Parcel instantiates these fields by name when values cross a Binder call.
-keepclassmembers class * implements android.os.Parcelable {
    public static final android.os.Parcelable$Creator CREATOR;
}

# These are compile-time annotations and unrelated basement UI dependencies.
-dontwarn androidx.**
-dontwarn com.google.android.gms.common.R**
-dontwarn javax.annotation.**
-dontwarn com.google.errorprone.annotations.**
-dontwarn org.checkerframework.**
