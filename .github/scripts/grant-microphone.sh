#!/bin/bash
# Grants this job's runner the microphone on Depot's macOS runners. AVAudioEngine and AVPlayer open the default device's input as well, and its runner, started from a directory of its own per job, is not among those the image grants, so the first sound waits on a prompt no one answers. SIP is off there, so the TCC databases can be written.
set -euo pipefail
now=$(date +%s)
for db in "/Library/Application Support/com.apple.TCC/TCC.db" "$HOME/Library/Application Support/com.apple.TCC/TCC.db"; do
	for runner in /Users/runner/runner-*/bin/Runner.Listener /Users/runner/runner-*/bin/Runner.Worker; do
		for service in kTCCServiceMicrophone kTCCServiceAudioCapture; do
			sudo sqlite3 "$db" "insert or replace into access (service, client, client_type, auth_value, auth_reason, auth_version, indirect_object_identifier, flags, last_modified, boot_uuid, last_reminded) values ('$service', '$runner', 1, 2, 4, 1, 'UNUSED', 0, $now, 'UNUSED', 0);"
		done
	done
done
sudo killall tccd || true
