import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Release signing: env vars (CI) or keystore.properties (local, gitignored).
val keystoreProps = Properties().apply {
    val f = rootProject.file("keystore.properties")
    if (f.exists()) f.inputStream().use { load(it) }
}

fun signingValue(env: String, prop: String): String? =
    System.getenv(env)?.takeIf { it.isNotBlank() } ?: keystoreProps.getProperty(prop)

val releaseStoreFile = signingValue("TYCHE_KEYSTORE_FILE", "storeFile")

android {
    namespace = "com.omareloui.tyche"
    compileSdk = 35
    // Matches the SDK in flake.nix, the nix store SDK is read only.
    buildToolsVersion = "35.0.0"

    defaultConfig {
        applicationId = "com.omareloui.tyche"
        minSdk = 26
        targetSdk = 35
        // CI passes -PversionCode / -PversionName so every release upgrades the last.
        versionCode = (findProperty("versionCode") as String?)?.toInt() ?: 1
        versionName = (findProperty("versionName") as String?) ?: "0.1.0"
    }

    signingConfigs {
        if (releaseStoreFile != null) {
            create("release") {
                storeFile = rootProject.file(releaseStoreFile)
                storePassword = signingValue("TYCHE_KEYSTORE_PASSWORD", "storePassword")
                keyAlias = signingValue("TYCHE_KEY_ALIAS", "keyAlias")
                keyPassword = signingValue("TYCHE_KEY_PASSWORD", "keyPassword")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfigs.findByName("release")?.let { signingConfig = it }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("androidx.browser:browser:1.8.0")
    implementation("androidx.work:work-runtime-ktx:2.10.0")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.7")
}
