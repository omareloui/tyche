# Tyche SMS (Android)

Minimal app that signs in to Tyche and forwards bank SMS to `POST /api/sms`,
where they become transactions flagged for review.

## Build

`flake.nix` provides JDK 17, Gradle and the Android SDK (`path:.` lets it work
while untracked by git):

```bash
nix develop path:.
./gradlew assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

The SDK lives in the read only nix store, so its versions in `flake.nix` must
match `compileSdk`/`buildToolsVersion` in `app/build.gradle.kts`.

Not on the Play Store, SMS permissions are restricted there, so sideload it.

## Releases

`.github/workflows/mobile-release.yml` builds a signed release APK on every push
to `main` or `dev` touching `mobile/` and publishes it as a GitHub release
`mobile-v0.1.<run>` (prerelease from `dev`). The run number is the
`versionCode`, so each release installs over the previous one.

It signs with a keystore from the repo secrets `ANDROID_KEYSTORE_BASE64`,
`ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS` and `ANDROID_KEY_PASSWORD`.
Create it once (keep it safe, losing it means uninstalling to upgrade):

```bash
keytool -genkeypair -v -keystore tyche-release.jks -keyalg RSA -keysize 4096 -validity 10000 -alias tyche
base64 -w0 tyche-release.jks | gh secret set ANDROID_KEYSTORE_BASE64
gh secret set ANDROID_KEYSTORE_PASSWORD
gh secret set ANDROID_KEY_ALIAS --body tyche
gh secret set ANDROID_KEY_PASSWORD
```

Local release builds read the same from `keystore.properties` (gitignored):
`storeFile`, `storePassword`, `keyAlias`, `keyPassword`. Debug and release
builds are signed with different keys, so switching needs one uninstall.

## Flow

1. Enter the Tyche URL and sign in. The app opens `/oauth/mobile` with a PKCE
   challenge in a Custom Tab, Tyche redirects to `tyche://auth?code=...` and the
   app exchanges the code + verifier at `/api/mobile/token` for a device token
   (stored encrypted with a Keystore key). The app also sends a stable install
   id, so signing in again reuses the same device on Tyche (and un-revokes it)
   with a new token, the old one stops working.
2. `SmsReceiver` queues SMS from the allowed senders (default HSBC, CIB, AAIB)
   in a local SQLite queue and `UploadWorker` uploads them when online.
   `CatchUpWorker` runs every 6 hours (and on opening the app or signing in)
   to queue the bank SMS the receiver missed (app force stopped, killed by the
   battery saver, signed out) from the inbox since its last run, leaving out
   the ones already uploaded (`sent_sms`, kept 30 days), then restarts the
   upload if anything waits. After SMS access, the app asks once to be left
   out of battery optimization; the status card keeps asking while it isn't.
3. Everything the app does (SMS queued or ignored, uploads and what Tyche did
   with each message, failures with their cause) goes to an activity log on the
   main screen, also in logcat under the `Tyche` tag. The status card on top
   says whether SMS get through, and when they're stuck (waiting over 10
   minutes, a failing server, a rejected device) why and since when.
4. Unparsed or unmatched SMS show on Tyche's `/sms` page, where devices can be
   revoked too. Set an account's "SMS Reference" for the matching.

## Testing against the local server over USB

Debug builds allow plain http. `adb reverse` makes the phone's `localhost:<port>`
reach the laptop, so `OAUTH_CB_ORIGIN=http://localhost:<port>` works for both
(Google only allows plain http redirects for localhost):

```bash
adb reverse tcp:<port> tcp:<port>
```

Then use `http://localhost:<port>` as the Tyche URL in the app.

## Dev mode

Tap the `tyche` header 7 times (any build) to toggle it. A "Dev tools" button
then opens a screen sending fake bank SMS through the receiver's path (sender
check, queue, activity log, upload), to test the parser, `/sms`, reviews and
the PWA badge. Presets (`DevSms.kt`, from the samples in
`internal/smsparser/seed.go`): transactions dated now with a random amount,
ignored messages and unrecognized ones; the fields stay editable. The account
reference is filled into the transaction SMS: set it as an account's SMS
Reference on Tyche, another one tests unmatched SMS. Each send is stamped now,
so the same preset is never a duplicate. Fakes are real data on the signed-in
Tyche, nothing goes to the phone's inbox.

## Testing on the emulator

```bash
adb emu sms send HSBC "From HSBC: 26AUG25 SHOP Purchase from 123-456***-789 EGP 220.00- Your available balance is EGP 4,554.36"
```
