#!/usr/bin/env python3
"""Fetch pinned Play Review bytecode for build.sh, never for an app's build."""

import hashlib
import io
import json
from pathlib import Path
import sys
import urllib.request
import zipfile

# Google's Maven artifacts and the transitive versions in their POMs.
ARTIFACTS = {
    "com/google/android/play/review/2.0.2/review-2.0.2": "1602026678231dba0f7482d1244e45ff83d9ff22fc8b6d6be8961228ac7a32dd",
    "com/google/android/play/core-common/2.0.4/core-common-2.0.4": "36400824014fc61d9379b13dd1ab13f6514847fb04347c0fdfe618bace78f3ed",
    "com/google/android/gms/play-services-tasks/18.2.0/play-services-tasks-18.2.0": "7f2aaa8f502068eaf54356ca92aec04271d6e7c416c52c45c0d23440fcbd1654",
    "com/google/android/gms/play-services-basement/18.4.0/play-services-basement-18.4.0": "ce5c936fd66814b3602f5c6a5e929911ff973d4b05f6de9996da596bef97cad2",
}


def fetch(destination):
    destination.mkdir(parents=True, exist_ok=True)
    notices = set()
    for artifact, checksum in ARTIFACTS.items():
        url = "https://dl.google.com/dl/android/maven2/" + artifact + ".aar"
        with urllib.request.urlopen(url, timeout=60) as response:
            data = response.read()
        if hashlib.sha256(data).hexdigest() != checksum:
            raise ValueError("Play Review checksum mismatch: " + artifact)
        with zipfile.ZipFile(io.BytesIO(data)) as aar:
            # Only classes are compiler inputs. The app's manifest uses a
            # framework translucent theme for the SDK's empty wrapper Activity.
            (destination / (artifact.rsplit("/", 1)[1] + ".jar")).write_bytes(aar.read("classes.jar"))
            if "LICENSE" in aar.namelist():
                notices.add(aar.read("LICENSE").decode().strip())
            raw = aar.read("third_party_licenses.txt")
            for label, span in json.loads(aar.read("third_party_licenses.json")).items():
                notice = raw[span["start"]:span["start"] + span["length"]].decode().strip()
                notices.add(label + "\n\n" + notice)
    (destination / "NOTICE.txt").write_text("Play Review and its dependencies\n\n" + "\n\n".join(sorted(notices)) + "\n")


if __name__ == "__main__":
    fetch(Path(sys.argv[1]))
