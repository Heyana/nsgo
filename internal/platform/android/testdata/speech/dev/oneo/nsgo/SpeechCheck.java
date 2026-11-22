package dev.oneo.nsgo;

import android.app.Activity;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.Bundle;
import android.speech.RecognizerIntent;
import android.speech.SpeechRecognizer;
import java.util.ArrayList;
import java.util.Arrays;

/** Exercises the real Speech.java on the JVM against minimal Android speech stubs. */
public final class SpeechCheck {
    public static void main(String[] args) {
        availability();
        missingManifestPermission();
        permissionPromptFlow();
        stopWhileWaitingPermission();
        abortAndReplace();
        errorMapping();
    }

    private static void availability() {
        Activity act = new Activity();
        SpeechRecognizer.recognitionAvailable = false;
        SpeechRecognizer.onDeviceAvailable = false;
        check(!Speech.available(act), "expected unavailable");

        SpeechRecognizer.onDeviceAvailable = true;
        Build.VERSION.SDK_INT = 30;
        check(!Speech.available(act), "on-device requires API 31+");
        Build.VERSION.SDK_INT = 31;
        check(Speech.available(act), "expected on-device available on API 31+");

        SpeechRecognizer.recognitionAvailable = true;
        SpeechRecognizer.onDeviceAvailable = false;
        check(Speech.available(act), "expected cloud recognition available");
    }

    private static void missingManifestPermission() {
        SpeechRecognizer.recognitionAvailable = true;
        SpeechRecognizer.onDeviceAvailable = false;
        Activity act = new Activity();
        act.pm.info.requestedPermissions = new String[] {"android.permission.INTERNET"};
        NsgoView view = new NsgoView(act);
        Speech.handle(view, 1, Speech.START, "en-US", true);
        check(act.requestCalls == 0, "should not prompt when manifest omits RECORD_AUDIO");
        check(view.events.equals(Arrays.asList("1:2::false:not-allowed")), "events=" + view.events);
    }

    private static void permissionPromptFlow() {
        Activity act = new Activity();
        act.pm.info.requestedPermissions = new String[] {Speech.PERMISSION};
        act.selfPermission = PackageManager.PERMISSION_DENIED;
        NsgoView view = new NsgoView(act);

        // Denied permission ends not-allowed.
        Speech.handle(view, 2, Speech.START, "en-US", true);
        check(act.requestCalls == 1 && act.requestedCode == Speech.PERMISSION_REQUEST, "expected permission prompt");
        Speech.permissionResult(view, false);
        check(view.events.equals(Arrays.asList("2:2::false:not-allowed")), "denied events=" + view.events);

        // Granted permission starts recognizer, reports ready, partial, stop, final, end.
        view.events.clear();
        Speech.handle(view, 3, Speech.START, "en-US", true);
        check(act.requestCalls == 2, "expected second prompt");
        act.selfPermission = PackageManager.PERMISSION_GRANTED;
        Speech.permissionResult(view, true);
        SpeechRecognizer rec = SpeechRecognizer.lastCreated;
        check(rec != null && rec.starts == 1, "recognizer did not start");
        check("en-US".equals(rec.lastIntent.extras.get(RecognizerIntent.EXTRA_LANGUAGE)), "lang=" + rec.lastIntent.extras);
        check(Boolean.TRUE.equals(rec.lastIntent.extras.get(RecognizerIntent.EXTRA_PARTIAL_RESULTS)), "interim=" + rec.lastIntent.extras);

        rec.listener.onReadyForSpeech(new Bundle());
        rec.listener.onPartialResults(bundleOf("hello"));
        Speech.handle(view, 3, Speech.STOP, "", false);
        check(rec.stops == 1, "stopListening not called");
        rec.listener.onResults(bundleOf("hello world"));
        check(rec.destroys == 1, "recognizer not destroyed");
        check(view.events.equals(Arrays.asList(
                "3:0::false:",
                "3:1:hello:false:",
                "3:1:hello world:true:",
                "3:2:hello world:false:"
        )), "full events=" + view.events);

        // Interim: false suppresses partial results.
        view.events.clear();
        Speech.handle(view, 4, Speech.START, "ja-JP", false);
        SpeechRecognizer rec4 = SpeechRecognizer.lastCreated;
        rec4.listener.onReadyForSpeech(new Bundle());
        rec4.listener.onPartialResults(bundleOf("ignored"));
        rec4.listener.onResults(bundleOf("final text"));
        check(view.events.equals(Arrays.asList(
                "4:0::false:",
                "4:1:final text:true:",
                "4:2:final text:false:"
        )), "non-interim events=" + view.events);
    }

    private static void stopWhileWaitingPermission() {
        Activity act = new Activity();
        act.pm.info.requestedPermissions = new String[] {Speech.PERMISSION};
        act.selfPermission = PackageManager.PERMISSION_DENIED;
        NsgoView view = new NsgoView(act);
        SpeechRecognizer.lastCreated = null;

        Speech.handle(view, 5, Speech.START, "", true);
        Speech.handle(view, 5, Speech.STOP, "", false);
        Speech.permissionResult(view, true);
        check(SpeechRecognizer.lastCreated == null, "late grant started stopped session");
        check(view.events.equals(Arrays.asList("5:2::false:")), "stop while waiting=" + view.events);
    }

    private static void abortAndReplace() {
        Activity act = new Activity();
        act.pm.info.requestedPermissions = new String[] {Speech.PERMISSION};
        act.selfPermission = PackageManager.PERMISSION_GRANTED;
        NsgoView view = new NsgoView(act);

        Speech.handle(view, 6, Speech.START, "", true);
        SpeechRecognizer rec6 = SpeechRecognizer.lastCreated;
        Speech.handle(view, 6, Speech.ABORT, "", false);
        check(rec6.cancels == 1 && rec6.destroys == 1, "abort did not teardown");
        check(view.events.isEmpty(), "abort emitted events=" + view.events);

        Speech.handle(view, 7, Speech.START, "", true);
        SpeechRecognizer rec7 = SpeechRecognizer.lastCreated;
        Speech.handle(view, 8, Speech.START, "", true);
        SpeechRecognizer rec8 = SpeechRecognizer.lastCreated;
        check(rec7.cancels == 1 && rec7.destroys == 1, "replacement did not teardown old");
        Speech.releaseView(view);
        check(rec8.cancels == 1 && rec8.destroys == 1, "releaseView did not teardown");
    }

    private static void errorMapping() {
        check("audio-capture".equals(Speech.mapError(SpeechRecognizer.ERROR_AUDIO)), "ERROR_AUDIO");
        check("not-allowed".equals(Speech.mapError(SpeechRecognizer.ERROR_INSUFFICIENT_PERMISSIONS)), "ERROR_INSUFFICIENT_PERMISSIONS");
        check("network".equals(Speech.mapError(SpeechRecognizer.ERROR_NETWORK)), "ERROR_NETWORK");
        check("network".equals(Speech.mapError(SpeechRecognizer.ERROR_NETWORK_TIMEOUT)), "ERROR_NETWORK_TIMEOUT");
        check("network".equals(Speech.mapError(SpeechRecognizer.ERROR_SERVER)), "ERROR_SERVER");
        check("network".equals(Speech.mapError(SpeechRecognizer.ERROR_SERVER_DISCONNECTED)), "ERROR_SERVER_DISCONNECTED");
        check("no-speech".equals(Speech.mapError(SpeechRecognizer.ERROR_NO_MATCH)), "ERROR_NO_MATCH");
        check("no-speech".equals(Speech.mapError(SpeechRecognizer.ERROR_SPEECH_TIMEOUT)), "ERROR_SPEECH_TIMEOUT");
        check("language-not-supported".equals(Speech.mapError(SpeechRecognizer.ERROR_LANGUAGE_NOT_SUPPORTED)), "ERROR_LANGUAGE_NOT_SUPPORTED");
        check("language-not-supported".equals(Speech.mapError(SpeechRecognizer.ERROR_LANGUAGE_UNAVAILABLE)), "ERROR_LANGUAGE_UNAVAILABLE");
        check("failed".equals(Speech.mapError(SpeechRecognizer.ERROR_CLIENT)), "ERROR_CLIENT");
        check("failed".equals(Speech.mapError(SpeechRecognizer.ERROR_RECOGNIZER_BUSY)), "ERROR_RECOGNIZER_BUSY");
    }

    private static Bundle bundleOf(String... items) {
        Bundle b = new Bundle();
        b.putStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION, new ArrayList<>(Arrays.asList(items)));
        return b;
    }

    private static void check(boolean ok, String msg) {
        if (!ok) throw new AssertionError(msg);
    }
}
