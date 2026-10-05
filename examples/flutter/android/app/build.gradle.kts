plugins {
    id("com.android.application")
    id("kotlin-android")
    id("dev.flutter.flutter-gradle-plugin")
}

// 仅消费本次显式环境；不创建明文签名 properties，也不回退 debug key。
fun signingValue(name: String): String =
    requireNotNull(System.getenv(name)?.takeIf { it.isNotEmpty() }) { "缺少明确签名声明" }

android {
    namespace = "dev.mybuilds.mvp_flutter"
    compileSdk = flutter.compileSdkVersion
    ndkVersion = flutter.ndkVersion
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = JavaVersion.VERSION_17.toString() }
    defaultConfig {
        applicationId = "dev.mybuilds.mvp_flutter"
        minSdk = flutter.minSdkVersion
        targetSdk = flutter.targetSdkVersion
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }
    signingConfigs {
        create("explicitRelease") {
            storeFile = file(signingValue("ANDROID_KEYSTORE"))
            keyAlias = signingValue("ANDROID_KEY_ALIAS")
            storePassword = signingValue("ANDROID_KEYSTORE_PASSWORD")
            keyPassword = signingValue("ANDROID_KEY_PASSWORD")
        }
    }
    // 空 flavor 保留默认工程任务，非空时启用已明确准备的两个变体。
    if (!System.getenv("APP_FLAVOR").isNullOrEmpty()) {
        flavorDimensions += "environment"
        productFlavors {
            create("production") { dimension = "environment" }
            create("staging") {
                dimension = "environment"
                applicationIdSuffix = ".staging"
                resValue("string", "app_name", "mybuilds staging")
            }
        }
    }
    buildTypes {
        release {
            signingConfig = signingConfigs.getByName("explicitRelease")
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }
}
flutter { source = "../.." }
