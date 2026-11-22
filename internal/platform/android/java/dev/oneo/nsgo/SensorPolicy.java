package dev.oneo.nsgo;

/** Decisions shared by framework adapters and JVM fixtures. No Android dependency. */
final class SensorPolicy {
    // Wire kind/command pairs are explicit: a future permission is never location.
    static boolean validRequest(int command,int kind) {
        if(kind<0||kind>7)return false;
        switch(command){
            case 0:case 1:case 5:return true;
            case 2:case 3:return kind==0||kind==1;
            case 4:case 11:return kind==2;
            case 6:return kind==3;
            case 7:return kind==4;
            case 8:return kind==5;
            case 9:return kind==6;
            case 10:return kind==0;
            default:return false;
        }
    }

    static String provider(int api, boolean fine, boolean fused, boolean gps, boolean network, double accuracy) {
        if (api >= 31 && fused) return "fused";
        if (accuracy > 100 && network) return "network";
        if (fine && gps) return "gps";
        if (network) return "network";
        return null;
    }

    static boolean fresh(long sampleNanos, long startedNanos, long nowNanos, long maxAge) {
        return sampleNanos <= nowNanos && (sampleNanos >= startedNanos || maxAge > 0 && nowNanos-sampleNanos <= maxAge);
    }

    static int period(long intervalNanos, int minimumMicros) {
        return Math.max(minimumMicros, (int)Math.max(10000L, Math.min(1000000L, intervalNanos/1000L)));
    }
}
