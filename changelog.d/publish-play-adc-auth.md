### Added

- Fall back to Google Application Default Credentials in `nsgo publish` for Google Play when `NSGO_PLAY_SERVICE_ACCOUNT` is unset, supporting service account impersonation (`gcloud auth application-default login --impersonate-service-account=...`), Workload Identity Federation (`google-github-actions/auth`), `GOOGLE_APPLICATION_CREDENTIALS`, and GCE metadata, while reporting the credential identity in output and 401/403 errors (#2600).
