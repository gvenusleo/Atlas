// Plugins pinned below the Flutter compileSdk (36) fail the build when their
// transitive androidx dependencies require a newer API level (clipboard 3.0.14
// compiles against 33). Lift every Android library plugin to the app's
// compileSdk after its own evaluation, so nothing can override it back.
// Registered before any subproject is evaluated.
subprojects {
    afterEvaluate(closureOf<Project> {
        extensions.findByType(com.android.build.gradle.BaseExtension::class.java)
            ?.let { it.compileSdkVersion(36) }
    })
}

allprojects {
    repositories {
        google()
        mavenCentral()
    }
}

val newBuildDir: Directory =
    rootProject.layout.buildDirectory
        .dir("../../build")
        .get()
rootProject.layout.buildDirectory.value(newBuildDir)

subprojects {
    val newSubprojectBuildDir: Directory = newBuildDir.dir(project.name)
    project.layout.buildDirectory.value(newSubprojectBuildDir)
}
subprojects {
    project.evaluationDependsOn(":app")
}

tasks.register<Delete>("clean") {
    delete(rootProject.layout.buildDirectory)
}
