// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "GrimlockerSDK",
    platforms: [.iOS(.v15), .macOS(.v12), .watchOS(.v8), .tvOS(.v15)],
    products: [
        .library(name: "GrimlockerSDK", targets: ["GrimlockerSDK"]),
    ],
    targets: [
        .target(name: "GrimlockerSDK", dependencies: [], path: "Sources/GrimlockerSDK"),
        .testTarget(name: "GrimlockerSDKTests", dependencies: ["GrimlockerSDK"]),
    ]
)
