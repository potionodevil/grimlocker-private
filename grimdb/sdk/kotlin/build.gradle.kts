plugins {
    kotlin("jvm") version "2.0.0"
    `maven-publish`
}

group = "com.grimlocker"
version = "1.0.0"

repositories {
    mavenCentral()
}

dependencies {
    implementation("com.google.code.gson:gson:2.11.0")
    testImplementation(kotlin("test"))
    testImplementation("org.junit.jupiter:junit-jupiter:5.10.2")
}

tasks.test {
    useJUnitPlatform()
}

kotlin {
    jvmToolchain(17)
}

publishing {
    publications {
        create<MavenPublication>("maven") {
            groupId = "com.grimlocker"
            artifactId = "grimlocker-sdk-kotlin"
            version = "1.0.0"
            from(components["java"])
        }
    }
}
