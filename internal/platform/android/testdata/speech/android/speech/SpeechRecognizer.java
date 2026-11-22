package android.speech;

import android.content.Context;
import android.content.Intent;

public class SpeechRecognizer {
    public static final String RESULTS_RECOGNITION = "results_recognition";
    public static final int ERROR_NETWORK_TIMEOUT = 1;
    public static final int ERROR_NETWORK = 2;
    public static final int ERROR_AUDIO = 3;
    public static final int ERROR_SERVER = 4;
    public static final int ERROR_CLIENT = 5;
    public static final int ERROR_SPEECH_TIMEOUT = 6;
    public static final int ERROR_NO_MATCH = 7;
    public static final int ERROR_RECOGNIZER_BUSY = 8;
    public static final int ERROR_INSUFFICIENT_PERMISSIONS = 9;
    public static final int ERROR_TOO_MANY_REQUESTS = 10;
    public static final int ERROR_SERVER_DISCONNECTED = 11;
    public static final int ERROR_LANGUAGE_NOT_SUPPORTED = 12;
    public static final int ERROR_LANGUAGE_UNAVAILABLE = 13;

    public static boolean recognitionAvailable = true;
    public static boolean onDeviceAvailable = false;
    public static SpeechRecognizer lastCreated;

    public RecognitionListener listener;
    public Intent lastIntent;
    public int starts, stops, cancels, destroys;
    public boolean onDevice;

    public static boolean isRecognitionAvailable(Context context) { return recognitionAvailable; }
    public static boolean isOnDeviceRecognitionAvailable(Context context) { return onDeviceAvailable; }

    public static SpeechRecognizer createSpeechRecognizer(Context context) {
        SpeechRecognizer r = new SpeechRecognizer();
        lastCreated = r;
        return r;
    }

    public static SpeechRecognizer createOnDeviceSpeechRecognizer(Context context) {
        SpeechRecognizer r = new SpeechRecognizer();
        r.onDevice = true;
        lastCreated = r;
        return r;
    }

    public void setRecognitionListener(RecognitionListener l) { this.listener = l; }
    public void startListening(Intent intent) { this.lastIntent = intent; this.starts++; }
    public void stopListening() { this.stops++; }
    public void cancel() { this.cancels++; }
    public void destroy() { this.destroys++; }
}
