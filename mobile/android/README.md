# Tyche Android

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

## Flow

1. Enter the Tyche URL and sign in. The app opens `/oauth/mobile` with a PKCE
   challenge in a Custom Tab, Tyche redirects to `tyche://auth?code=...` and the
   app exchanges the code + verifier at `/api/mobile/token` for a device token
   (stored encrypted with a Keystore key).
2. `SmsReceiver` queues SMS from the allowed senders (default HSBC, CIB, AAIB)
   in a local SQLite queue and `UploadWorker` uploads them when online.
3. Unparsed or unmatched SMS show on Tyche's `/sms` page, where devices can be
   revoked too. Set an account's "SMS Reference" for the matching.

## Testing against the local server over USB

Debug builds allow plain http. `adb reverse` makes the phone's `localhost:<port>`
reach the laptop, so `OAUTH_CB_ORIGIN=http://localhost:<port>` works for both
(Google only allows plain http redirects for localhost):

```bash
adb reverse tcp:<port> tcp:<port>
```

Then use `http://localhost:<port>` as the Tyche URL in the app.

## Testing on the emulator

```bash
adb emu sms send HSBC "From HSBC: 26AUG25 SHOP Purchase from 123-456***-789 EGP 220.00- Your available balance is EGP 4,554.36"
```
