plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

val clocheVersion = providers.fileContents(rootProject.layout.projectDirectory.file("../VERSION")).asText.get().trim()
val clocheVersionCode = clocheVersion.split(".").let { parts ->
    require(parts.size == 3 && parts.all { p -> p.toIntOrNull()?.let { it in 0..999 } == true }) { "VERSION must be X.Y.Z, each 0 to 999: $clocheVersion" }
    parts.fold(0) { code, p -> code * 1000 + p.toInt() }
}

android {
    namespace = "sh.ahmd.cloche"
    compileSdk {
        version = release(37) { minorApiLevel = 2 }
    }

    defaultConfig {
        applicationId = "sh.ahmd.cloche"
        minSdk = 34
        targetSdk = 37
        versionCode = clocheVersionCode
        versionName = clocheVersion
    }

    val releaseKey = System.getenv("CLOCHE_KEYSTORE")?.let(::file)
    signingConfigs {
        if (releaseKey != null) {
            create("release") {
                storeFile = releaseKey
                storePassword = System.getenv("CLOCHE_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("CLOCHE_KEY_ALIAS") ?: "cloche"
                keyPassword = System.getenv("CLOCHE_KEY_PASSWORD") ?: System.getenv("CLOCHE_KEYSTORE_PASSWORD")
                enableV3Signing = true
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"))
            signingConfig = signingConfigs.getByName(if (releaseKey != null) "release" else "debug")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    testOptions {
        unitTests.all { it.systemProperty("testdata", rootProject.file("../testdata").absolutePath) }
    }
}

dependencies {
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.foundation)
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.serialization.json)
    debugImplementation(libs.androidx.compose.ui.tooling)
    testImplementation(libs.junit4)
}
