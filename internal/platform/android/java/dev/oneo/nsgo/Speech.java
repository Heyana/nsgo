package dev.oneo.nsgo;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.Bundle;
import android.speech.RecognitionListener;
import android.speech.RecognizerIntent;
import android.speech.SpeechRecognizer;
import android.util.Log;
import java.util.ArrayList;
import java.util.Arrays;

/**
 * Recognizing speech through Android's SpeechRecognizer, as core's StartSpeech: one session in the process at a time, a new one aborting the one before. Requests RECORD_AUDIO from the activity when declared in the manifest and not yet granted; without RECORD_AUDIO in the manifest a session ends not-allowed. Its state is the main thread's.
 */
final class Speech {
    private Speech() {}

    static final int PERMISSION_REQUEST = 0x6e7d;
    static final String PERMISSION = "android.permission.RECORD_AUDIO";

    // What ITEM_SPEECH asks, as driver.SpeechCommand.
    static final int START = 0, STOP = 1, ABORT = 2;
    // What EVENT_SPEECH reports, as driver.SpeechPhase.
    static final int STARTED = 0, RESULT = 1, ENDED = 2;

    private static Session current;

    private static final class Session {
        final NsgoView view;
        final long count;
        final String lang;
        final boolean interim;
        SpeechRecognizer recognizer;
        String text = "";
        boolean started;
        boolean waitingPermission;
        boolean done;

        Session(NsgoView view, long count, String lang, boolean interim) {
            this.view = view;
            this.count = count;
            this.lang = lang == null ? "" : lang;
            this.interim = interim;
        }
    }

    /**
     * Whether the device has a speech recognition service, which Environment.Speech reports; an app targeting API 30 or later sees the RecognitionService only with the query the runtime's manifest declares.
     */
    static boolean available(Context context) {
        try {
            if (SpeechRecognizer.isRecognitionAvailable(context)) return true;
            return Build.VERSION.SDK_INT >= 31 && SpeechRecognizer.isOnDeviceRecognitionAvailable(context);
        } catch (RuntimeException e) {
            return false;
        }
    }

    private static boolean declaresRecordAudio(Context context) {
        try {
            String[] asked = context.getPackageManager()
                    .getPackageInfo(context.getPackageName(), PackageManager.GET_PERMISSIONS)
                    .requestedPermissions;
            return asked != null && Arrays.asList(asked).contains(PERMISSION);
        } catch (PackageManager.NameNotFoundException e) {
            return false;
        }
    }

    /** Carries out ITEM_SPEECH count of view, on the main thread. */
    static void handle(NsgoView view, long count, int command, String lang, boolean interim) {
        switch (command) {
            case START: {
                if (current != null) teardown(current);
                Session s = new Session(view, count, lang, interim);
                current = s;
                Context ctx = view.getContext();
                if (!available(ctx)) {
                    finish(s, "unavailable");
                    return;
                }
                if (!declaresRecordAudio(ctx)) {
                    Log.w("NSGo", "nsgo: speech needs android.permission.RECORD_AUDIO in the app's manifest (nsgo.yaml's android.permissions)");
                    finish(s, "not-allowed");
                    return;
                }
                if (ctx.checkSelfPermission(PERMISSION) != PackageManager.PERMISSION_GRANTED) {
                    Activity activity = view.activity();
                    if (activity == null) {
                        finish(s, "not-allowed");
                        return;
                    }
                    s.waitingPermission = true;
                    activity.requestPermissions(new String[] {PERMISSION}, PERMISSION_REQUEST);
                    return;
                }
                begin(s);
                break;
            }
            case STOP: {
                Session s = current;
                if (s == null || s.view != view || s.count != count || s.done) return;
                if (s.waitingPermission || s.recognizer == null) {
                    finish(s, "");
                    return;
                }
                try {
                    s.recognizer.stopListening();
                } catch (RuntimeException e) {
                    finish(s, "");
                }
                break;
            }
            case ABORT: {
                Session s = current;
                if (s != null && s.view == view && s.count == count) {
                    teardown(s);
                }
                break;
            }
        }
    }

    /** Hears the activity's RECORD_AUDIO permission prompt answer for speech recognition. */
    static void permissionResult(NsgoView view, boolean granted) {
        Session s = current;
        if (s == null || s.view != view || !s.waitingPermission || s.done) return;
        s.waitingPermission = false;
        if (!granted) {
            finish(s, "not-allowed");
            return;
        }
        begin(s);
    }

    /** Tears down any active session owned by view when the view stops or detaches. */
    static void releaseView(NsgoView view) {
        if (current != null && current.view == view) {
            teardown(current);
        }
    }

    private static void begin(Session s) {
        if (s.done || current != s) return;
        Context ctx = s.view.getContext();
        SpeechRecognizer recognizer = createRecognizer(ctx);
        if (recognizer == null) {
            finish(s, "unavailable");
            return;
        }
        s.recognizer = recognizer;
        recognizer.setRecognitionListener(new RecognitionListener() {
            @Override public void onReadyForSpeech(Bundle params) {
                markStarted(s);
            }

            @Override public void onBeginningOfSpeech() {
                markStarted(s);
            }

            @Override public void onRmsChanged(float rmsdB) {}

            @Override public void onBufferReceived(byte[] buffer) {}

            @Override public void onEndOfSpeech() {}

            @Override public void onError(int error) {
                if (s.done || current != s) return;
                finish(s, mapError(error));
            }

            @Override public void onResults(Bundle results) {
                if (s.done || current != s) return;
                String text = bestResult(results);
                if (!text.isEmpty()) {
                    s.text = text;
                    s.view.answerSpeech(s.count, RESULT, text, true, "");
                    finish(s, "");
                } else if (!s.text.isEmpty()) {
                    s.view.answerSpeech(s.count, RESULT, s.text, true, "");
                    finish(s, "");
                } else {
                    finish(s, "no-speech");
                }
            }

            @Override public void onPartialResults(Bundle partialResults) {
                if (s.done || current != s || !s.interim) return;
                String text = bestResult(partialResults);
                if (text.isEmpty()) return;
                s.text = text;
                s.view.answerSpeech(s.count, RESULT, text, false, "");
            }

            @Override public void onEvent(int eventType, Bundle params) {}
        });

        Intent intent = new Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH);
        intent.putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM);
        intent.putExtra(RecognizerIntent.EXTRA_PARTIAL_RESULTS, s.interim);
        intent.putExtra(RecognizerIntent.EXTRA_MAX_RESULTS, 1);
        if (!s.lang.isEmpty()) {
            intent.putExtra(RecognizerIntent.EXTRA_LANGUAGE, s.lang);
        }
        try {
            recognizer.startListening(intent);
        } catch (SecurityException e) {
            finish(s, "not-allowed");
        } catch (RuntimeException e) {
            finish(s, "failed");
        }
    }

    private static SpeechRecognizer createRecognizer(Context ctx) {
        try {
            if (SpeechRecognizer.isRecognitionAvailable(ctx)) {
                return SpeechRecognizer.createSpeechRecognizer(ctx);
            }
            if (Build.VERSION.SDK_INT >= 31 && SpeechRecognizer.isOnDeviceRecognitionAvailable(ctx)) {
                return SpeechRecognizer.createOnDeviceSpeechRecognizer(ctx);
            }
        } catch (RuntimeException ignored) {}
        return null;
    }

    private static void markStarted(Session s) {
        if (s.done || current != s || s.started) return;
        s.started = true;
        s.view.answerSpeech(s.count, STARTED, "", false, "");
    }

    private static void finish(Session s, String error) {
        if (s.done) return;
        String text = s.text;
        teardown(s);
        s.view.answerSpeech(s.count, ENDED, text, false, error);
    }

    private static void teardown(Session s) {
        s.done = true;
        s.waitingPermission = false;
        if (current == s) current = null;
        if (s.recognizer != null) {
            SpeechRecognizer r = s.recognizer;
            s.recognizer = null;
            try {
                r.setRecognitionListener(null);
                r.cancel();
                r.destroy();
            } catch (RuntimeException ignored) {}
        }
    }

    /** Extracts the highest-ranked non-empty transcript from a RecognitionListener Bundle. */
    static String bestResult(Bundle results) {
        if (results == null) return "";
        ArrayList<String> matches = results.getStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION);
        if (matches == null) return "";
        for (String match : matches) {
            if (match != null && !match.isEmpty()) return match;
        }
        return "";
    }

    /** Maps a SpeechRecognizer.ERROR_* code to driver's Web Speech API error name. */
    static String mapError(int error) {
        switch (error) {
            case SpeechRecognizer.ERROR_AUDIO:
                return "audio-capture";
            case SpeechRecognizer.ERROR_INSUFFICIENT_PERMISSIONS:
                return "not-allowed";
            case SpeechRecognizer.ERROR_NETWORK:
            case SpeechRecognizer.ERROR_NETWORK_TIMEOUT:
            case SpeechRecognizer.ERROR_SERVER:
            case SpeechRecognizer.ERROR_SERVER_DISCONNECTED:
                return "network";
            case SpeechRecognizer.ERROR_NO_MATCH:
            case SpeechRecognizer.ERROR_SPEECH_TIMEOUT:
                return "no-speech";
            case SpeechRecognizer.ERROR_LANGUAGE_NOT_SUPPORTED:
            case SpeechRecognizer.ERROR_LANGUAGE_UNAVAILABLE:
                return "language-not-supported";
            default:
                return "failed";
        }
    }
}
