package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func replaceOnce(text, old, replacement string) string {
	if strings.Contains(text, replacement) {
		return text
	}
	if !strings.Contains(text, old) {
		panic("Xcode patch anchor not found: " + old)
	}
	return strings.Replace(text, old, replacement, 1)
}

func main() {
	projectPath := filepath.Join("build", "ios", "xcode", "main.xcodeproj", "project.pbxproj")
	infoPlistPath := filepath.Join("build", "ios", "xcode", "main", "Info.plist")

	// Copy LiteRTLMServer.swift if present
	swiftLiteRTDest := filepath.Join("build", "ios", "xcode", "main", "LiteRTLMServer.swift")
	swiftLiteRTSrc := filepath.Join("build", "ios", "LiteRTLMServer.swift")
	if swiftData, err := os.ReadFile(swiftLiteRTSrc); err == nil {
		if err := os.WriteFile(swiftLiteRTDest, swiftData, 0o644); err != nil {
			panic(err)
		}
	}

	// Copy TranslationBridge.swift
	swiftTranslationDest := filepath.Join("build", "ios", "xcode", "main", "TranslationBridge.swift")
	swiftTranslationSrc := filepath.Join("build", "apple", "TranslationBridge.swift")
	swiftTransData, err := os.ReadFile(swiftTranslationSrc)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(swiftTranslationDest, swiftTransData, 0o644); err != nil {
		panic(err)
	}

	// Copy Assets.xcassets if present
	assetsSrc := filepath.Join("build", "ios", "Assets.xcassets")
	assetsDest := filepath.Join("build", "ios", "xcode", "main", "Assets.xcassets")
	if srcInfo, err := os.Stat(assetsSrc); err == nil && srcInfo.IsDir() {
		_ = filepath.Walk(assetsSrc, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(assetsSrc, path)
			target := filepath.Join(assetsDest, rel)
			if info.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, 0o644)
		})
	}

	// Wails 3 currently emits CFBundleExecutable=wailsapp even though the
	// generated Xcode target names its executable after PRODUCT_NAME. iOS then
	// rejects the otherwise valid bundle as "missing its bundle executable".
	infoPlistData, err := os.ReadFile(infoPlistPath)
	if err != nil {
		panic(err)
	}
	infoPlist := string(infoPlistData)
	infoPlist = replaceOnce(infoPlist,
		"<string>wailsapp</string>",
		"<string>$(EXECUTABLE_NAME)</string>")
	if !strings.Contains(infoPlist, "<key>CFBundleIconName</key>") {
		closingDict := strings.LastIndex(infoPlist, "</dict>")
		if closingDict >= 0 {
			iconEntry := `    <key>CFBundleIconName</key>
    <string>AppIcon</string>
`
			infoPlist = infoPlist[:closingDict] + iconEntry + infoPlist[closingDict:]
		}
	}
	if !strings.Contains(infoPlist, "<key>UIFileSharingEnabled</key>") {
		closingDict := strings.LastIndex(infoPlist, "</dict>")
		if closingDict >= 0 {
			fileSharingManifest := `    <key>UIFileSharingEnabled</key>
    <true/>
    <key>LSSupportsOpeningDocumentsInPlace</key>
    <true/>
`
			infoPlist = infoPlist[:closingDict] + fileSharingManifest + infoPlist[closingDict:]
		}
	}
	if !strings.Contains(infoPlist, "<key>UIApplicationSceneManifest</key>") {
		closingDict := strings.LastIndex(infoPlist, "</dict>")
		if closingDict < 0 {
			panic("generated iOS Info.plist has no closing dict")
		}
		sceneManifest := `    <key>UIApplicationSceneManifest</key>
    <dict>
        <key>UIApplicationSupportsMultipleScenes</key>
        <false/>
        <key>UISceneConfigurations</key>
        <dict>
            <key>UIWindowSceneSessionRoleApplication</key>
            <array>
                <dict>
                    <key>UISceneConfigurationName</key>
                    <string>Default Configuration</string>
                    <key>UISceneDelegateClassName</key>
                    <string>WailsSceneDelegate</string>
                </dict>
            </array>
        </dict>
    </dict>
`
		infoPlist = infoPlist[:closingDict] + sceneManifest + infoPlist[closingDict:]
	}

	// Enforce iPhone portrait only and iPad portrait + landscape
	if iphoneIdx := strings.Index(infoPlist, "<key>UISupportedInterfaceOrientations</key>"); iphoneIdx >= 0 {
		endArray := strings.Index(infoPlist[iphoneIdx:], "</array>")
		if endArray >= 0 {
			endIdx := iphoneIdx + endArray + len("</array>")
			iphoneReplacement := `<key>UISupportedInterfaceOrientations</key>
	<array>
		<string>UIInterfaceOrientationPortrait</string>
	</array>`
			infoPlist = infoPlist[:iphoneIdx] + iphoneReplacement + infoPlist[endIdx:]
		}
	}

	if ipadIdx := strings.Index(infoPlist, "<key>UISupportedInterfaceOrientations~ipad</key>"); ipadIdx >= 0 {
		endArray := strings.Index(infoPlist[ipadIdx:], "</array>")
		if endArray >= 0 {
			endIdx := ipadIdx + endArray + len("</array>")
			ipadReplacement := `<key>UISupportedInterfaceOrientations~ipad</key>
	<array>
		<string>UIInterfaceOrientationPortrait</string>
		<string>UIInterfaceOrientationPortraitUpsideDown</string>
		<string>UIInterfaceOrientationLandscapeLeft</string>
		<string>UIInterfaceOrientationLandscapeRight</string>
	</array>`
			infoPlist = infoPlist[:ipadIdx] + ipadReplacement + infoPlist[endIdx:]
		}
	}

	if err := os.WriteFile(infoPlistPath, []byte(infoPlist), 0o644); err != nil {
		panic(err)
	}

	raw, err := os.ReadFile(projectPath)
	if err != nil {
		panic(err)
	}
	text := string(raw)

	// PBXBuildFile section
	text = replaceOnce(text,
		"C0DEBEEF0000000000000001 /* main.m in Sources */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000002 /* main.m */; };",
		"C0DEBEEF0000000000000001 /* main.m in Sources */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000002 /* main.m */; };\n\t\tC0DEBEEF00000000000000F8 /* Assets.xcassets in Resources */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000108 /* Assets.xcassets */; };\n\t\tC0DEBEEF00000000000000F9 /* LaunchScreen.storyboard in Resources */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000109 /* LaunchScreen.storyboard */; };\n\t\tC0DEBEEF0000000000000201 /* LiteRTLMServer.swift in Sources */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000202 /* LiteRTLMServer.swift */; };\n\t\tC0DEBEEF0000000000000203 /* LiteRTLMNative in Frameworks */ = {isa = PBXBuildFile; productRef = C0DEBEEF0000000000000204 /* LiteRTLMNative */; };\n\t\tC0DEBEEF0000000000000211 /* TranslationBridge.swift in Sources */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000212 /* TranslationBridge.swift */; };\n\t\tC0DEBEEF0000000000000213 /* Translation.framework in Frameworks */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000214 /* Translation.framework */; settings = {ATTRIBUTES = (Weak, ); }; };\n\t\tC0DEBEEF0000000000000215 /* NaturalLanguage.framework in Frameworks */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000216 /* NaturalLanguage.framework */; };\n\t\tC0DEBEEF0000000000000217 /* SwiftUI.framework in Frameworks */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000218 /* SwiftUI.framework */; };")

	// PBXFileReference section
	text = replaceOnce(text,
		"C0DEBEEF0000000000000002 /* main.m */ = {isa = PBXFileReference; lastKnownFileType = sourcecode.c.objc; path = main.m; sourceTree = \"<group>\"; };",
		"C0DEBEEF0000000000000002 /* main.m */ = {isa = PBXFileReference; lastKnownFileType = sourcecode.c.objc; path = main.m; sourceTree = \"<group>\"; };\n\t\tC0DEBEEF0000000000000108 /* Assets.xcassets */ = {isa = PBXFileReference; lastKnownFileType = folder.assetcatalog; path = Assets.xcassets; sourceTree = \"<group>\"; };\n\t\tC0DEBEEF0000000000000109 /* LaunchScreen.storyboard */ = {isa = PBXFileReference; lastKnownFileType = file.storyboard; path = LaunchScreen.storyboard; sourceTree = \"<group>\"; };\n\t\tC0DEBEEF0000000000000202 /* LiteRTLMServer.swift */ = {isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = LiteRTLMServer.swift; sourceTree = \"<group>\"; };\n\t\tC0DEBEEF0000000000000212 /* TranslationBridge.swift */ = {isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = TranslationBridge.swift; sourceTree = \"<group>\"; };\n\t\tC0DEBEEF0000000000000214 /* Translation.framework */ = {isa = PBXFileReference; lastKnownFileType = wrapper.framework; name = Translation.framework; path = System/Library/Frameworks/Translation.framework; sourceTree = SDKROOT; };\n\t\tC0DEBEEF0000000000000216 /* NaturalLanguage.framework */ = {isa = PBXFileReference; lastKnownFileType = wrapper.framework; name = NaturalLanguage.framework; path = System/Library/Frameworks/NaturalLanguage.framework; sourceTree = SDKROOT; };\n\t\tC0DEBEEF0000000000000218 /* SwiftUI.framework */ = {isa = PBXFileReference; lastKnownFileType = wrapper.framework; name = SwiftUI.framework; path = System/Library/Frameworks/SwiftUI.framework; sourceTree = SDKROOT; };")

	// Group children
	text = replaceOnce(text,
		"C0DEBEEF0000000000000002 /* main.m */,\n\t\t\t\tC0DEBEEF0000000000000003 /* Info.plist */,",
		"C0DEBEEF0000000000000002 /* main.m */,\n\t\t\t\tC0DEBEEF0000000000000108 /* Assets.xcassets */,\n\t\t\t\tC0DEBEEF0000000000000109 /* LaunchScreen.storyboard */,\n\t\t\t\tC0DEBEEF0000000000000202 /* LiteRTLMServer.swift */,\n\t\t\t\tC0DEBEEF0000000000000212 /* TranslationBridge.swift */,\n\t\t\t\tC0DEBEEF0000000000000003 /* Info.plist */,")

	// Resources build phase
	resourcesSection := `/* Begin PBXResourcesBuildPhase section */
		C0DEBEEF0000000000000057 /* Resources */ = {
			isa = PBXResourcesBuildPhase;
			buildActionMask = 2147483647;
			files = (
				C0DEBEEF00000000000000F8 /* Assets.xcassets in Resources */,
				C0DEBEEF00000000000000F9 /* LaunchScreen.storyboard in Resources */,
			);
			runOnlyForDeploymentPostprocessing = 0;
		};
/* End PBXResourcesBuildPhase section */

/* Begin PBXShellScriptBuildPhase section */`
	text = replaceOnce(text,
		"/* Begin PBXShellScriptBuildPhase section */",
		resourcesSection)

	// Frameworks build phase
	text = replaceOnce(text,
		"C0DEBEEF00000000000000F7 /* DKST Translator AI.a in Frameworks */,",
		"C0DEBEEF00000000000000F7 /* DKST Translator AI.a in Frameworks */,\n\t\t\t\tC0DEBEEF0000000000000203 /* LiteRTLMNative in Frameworks */,\n\t\t\t\tC0DEBEEF0000000000000213 /* Translation.framework in Frameworks */,\n\t\t\t\tC0DEBEEF0000000000000215 /* NaturalLanguage.framework in Frameworks */,\n\t\t\t\tC0DEBEEF0000000000000217 /* SwiftUI.framework in Frameworks */,")

	// Sources build phase
	text = replaceOnce(text,
		"C0DEBEEF0000000000000001 /* main.m in Sources */,",
		"C0DEBEEF0000000000000001 /* main.m in Sources */,\n\t\t\t\tC0DEBEEF0000000000000201 /* LiteRTLMServer.swift in Sources */,\n\t\t\t\tC0DEBEEF0000000000000211 /* TranslationBridge.swift in Sources */,")

	// Embed dynamic framework run script phase
	embedScript := `/* Begin PBXShellScriptBuildPhase section */
		C0DEBEEF0000000000000208 /* Embed & Sign Dynamic Frameworks */ = {
			isa = PBXShellScriptBuildPhase;
			buildActionMask = 2147483647;
			files = (
			);
			inputFileListPaths = (
			);
			inputPaths = (
			);
			name = "Embed & Sign Dynamic Frameworks";
			outputFileListPaths = (
			);
			outputPaths = (
			);
			runOnlyForDeploymentPostprocessing = 0;
			shellPath = /bin/sh;
			shellScript = "FRAMEWORKS_DIR=\"${TARGET_BUILD_DIR}/${FRAMEWORKS_FOLDER_PATH}\"\nmkdir -p \"${FRAMEWORKS_DIR}\"\nSRC_FW=\"\"\nif [ -d \"${BUILT_PRODUCTS_DIR}/CLiteRTLM.framework\" ]; then\n  SRC_FW=\"${BUILT_PRODUCTS_DIR}/CLiteRTLM.framework\"\nelif [ -d \"${BUILT_PRODUCTS_DIR}/PackageFrameworks/CLiteRTLM.framework\" ]; then\n  SRC_FW=\"${BUILT_PRODUCTS_DIR}/PackageFrameworks/CLiteRTLM.framework\"\nelse\n  SRC_FW=$(find \"${BUILD_DIR}/..\" -name \"CLiteRTLM.framework\" -type d 2>/dev/null | grep -v \"${FRAMEWORKS_FOLDER_PATH}\" | head -n 1)\nfi\n\nif [ -n \"${SRC_FW}\" ] && [ -d \"${SRC_FW}\" ]; then\n  echo \"Embedding CLiteRTLM.framework from ${SRC_FW} to ${FRAMEWORKS_DIR}\"\n  rsync -a --delete \"${SRC_FW}\" \"${FRAMEWORKS_DIR}/\"\n  if [ -n \"${EXPANDED_CODE_SIGN_IDENTITY:-}\" ]; then\n    /usr/bin/codesign --force --sign \"${EXPANDED_CODE_SIGN_IDENTITY}\" --timestamp=none --preserve-metadata=identifier,entitlements,flags \"${FRAMEWORKS_DIR}/CLiteRTLM.framework\"\n  fi\nfi\n";
		};`
	text = replaceOnce(text,
		"/* Begin PBXShellScriptBuildPhase section */",
		embedScript)

	// Target buildPhases
	text = replaceOnce(text,
		"C0DEBEEF0000000000000056 /* Frameworks */,\n\t\t\t);",
		"C0DEBEEF0000000000000056 /* Frameworks */,\n\t\t\t\tC0DEBEEF0000000000000057 /* Resources */,\n\t\t\t\tC0DEBEEF0000000000000208 /* Embed & Sign Dynamic Frameworks */,\n\t\t\t);")

	// SPM Package references
	text = replaceOnce(text,
		"productReference = C0DEBEEF0000000000000004 /* DKST Translator AI.app */;",
		"packageProductDependencies = (\n\t\t\t\tC0DEBEEF0000000000000204 /* LiteRTLMNative */,\n\t\t\t);\n\t\t\tproductReference = C0DEBEEF0000000000000004 /* DKST Translator AI.app */;")
	text = replaceOnce(text,
		"projectRoot = \"\";\n\t\t\ttargets = (",
		"projectRoot = \"\";\n\t\t\tpackageReferences = (\n\t\t\t\tC0DEBEEF0000000000000205 /* XCLocalSwiftPackageReference LiteRTLMNative */,\n\t\t\t);\n\t\t\ttargets = (")
	text = replaceOnce(text,
		"/* Begin XCBuildConfiguration section */",
		"/* Begin XCLocalSwiftPackageReference section */\n\t\tC0DEBEEF0000000000000205 /* XCLocalSwiftPackageReference LiteRTLMNative */ = {isa = XCLocalSwiftPackageReference; relativePath = ../LiteRTLMNative; };\n/* End XCLocalSwiftPackageReference section */\n\n/* Begin XCSwiftPackageProductDependency section */\n\t\tC0DEBEEF0000000000000204 /* LiteRTLMNative */ = {isa = XCSwiftPackageProductDependency; package = C0DEBEEF0000000000000205 /* XCLocalSwiftPackageReference LiteRTLMNative */; productName = LiteRTLMNative; };\n/* End XCSwiftPackageProductDependency section */\n\n/* Begin XCBuildConfiguration section */")

	// Prebuild script
	text = replaceOnce(text,
		`export CGO_LDFLAGS=\"-isysroot ${SDK_PATH} -target ${GO_TARGET}\"\ncd \"${APP_ROOT}\"`,
		`export CGO_LDFLAGS=\"-isysroot ${SDK_PATH} -target ${GO_TARGET}\"\n# Xcode does not inherit the interactive shell PATH. Locate toolchains explicitly.\nexport PATH=\"/usr/local/go/bin:/opt/homebrew/bin:${HOME}/go/bin:${HOME}/.local/bin:${PATH}\"\nGO_BIN=\"${GO_BINARY:-}\"\nif [ -z \"${GO_BIN}\" ]; then GO_BIN=$(command -v go 2>/dev/null || true); fi\nif [ -z \"${GO_BIN}\" ] || [ ! -x \"${GO_BIN}\" ]; then\n  echo \"Go was not found. Install Go or set GO_BINARY to its absolute path in the Xcode scheme.\" >&2\n  exit 127\nfi\ncd \"${APP_ROOT}\"`)
	text = replaceOnce(text,
		`if [ ! -f build/ios/xcode/overlay.json ]; then\n  wails3 ios overlay:gen -out build/ios/xcode/overlay.json -config build/config.yml || true\nfi`,
		`if [ ! -f build/ios/xcode/overlay.json ]; then\n  WAILS_BIN=\"${WAILS3_BINARY:-}\"\n  if [ -z \"${WAILS_BIN}\" ]; then WAILS_BIN=$(command -v wails3 2>/dev/null || true); fi\n  if [ -z \"${WAILS_BIN}\" ] || [ ! -x \"${WAILS_BIN}\" ]; then\n    echo \"wails3 was not found. Install it or set WAILS3_BINARY to its absolute path in the Xcode scheme.\" >&2\n    exit 127\n  fi\n  \"${WAILS_BIN}\" ios overlay:gen -out build/ios/xcode/overlay.json -config build/config.yml\nfi`)
	text = replaceOnce(text,
		`go build -buildmode=c-archive -overlay build/ios/xcode/overlay.json -o \"bin/DKST Translator AI.a\"`,
		`\"${GO_BIN}\" build -buildmode=c-archive -overlay build/ios/xcode/overlay.json -o \"bin/DKST Translator AI.a\"`)

	// Build settings
	text = strings.ReplaceAll(text,
		"INFOPLIST_FILE = main/Info.plist;",
		`INFOPLIST_FILE = main/Info.plist;
				ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon;
				SUPPORTED_PLATFORMS = "iphoneos iphonesimulator";
				SUPPORTS_MAC_DESIGNED_FOR_IPHONE_IPAD = NO;
				SUPPORTS_MACCATALYST = NO;
				TARGETED_DEVICE_FAMILY = "1,2";
				ONLY_ACTIVE_ARCH = YES;
				ENABLE_USER_SCRIPT_SANDBOXING = NO;
				SWIFT_VERSION = 5.0;
				ALWAYS_SEARCH_USER_PATHS = NO;
				LD_RUNPATH_SEARCH_PATHS = (
					"$(inherited)",
					"@executable_path/Frameworks",
				);`)

	text = strings.ReplaceAll(text,
		"OTHER_LDFLAGS = (\n\t\t\t\t\t\"$(inherited)\",\n\t\t\t\t\t\"-ObjC\",\n\t\t\t\t);",
		"OTHER_LDFLAGS = (\n\t\t\t\t\t\"$(inherited)\",\n\t\t\t\t\t\"-ObjC\",\n\t\t\t\t\t\"-all_load\",\n\t\t\t\t);")

	text = strings.ReplaceAll(text,
		"IPHONEOS_DEPLOYMENT_TARGET = 15.0;",
		"IPHONEOS_DEPLOYMENT_TARGET = 15.5;")

	text = strings.ReplaceAll(text,
		"CODE_SIGNING_ALLOWED = NO;",
		"CODE_SIGN_STYLE = Automatic;\n\t\t\t\t\"CODE_SIGNING_ALLOWED[sdk=iphonesimulator*]\" = NO;")

	if err := os.WriteFile(projectPath, []byte(text), 0o644); err != nil {
		panic(err)
	}

	// main.m patching
	mainPath := filepath.Join("build", "ios", "xcode", "main", "main.m")
	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		panic(err)
	}
	mainText := string(mainData)
	mainText = replaceOnce(mainText, "#include <stdio.h>", "#include <stdio.h>\n#include <stdint.h>\n#include <dispatch/dispatch.h>\nextern void dkst_litertlm_server_start(void);\nextern int32_t DKSTAppleTranslationAvailable(void);\nextern int32_t DKSTGoogleTranslationAvailable(void);")
	mainText = replaceOnce(mainText,
		"extern void dkst_litertlm_server_start(void);",
		`extern void dkst_litertlm_server_start(void);

// iOS scene lifecycle delegate
@interface WailsSceneDelegate : UIResponder <UIWindowSceneDelegate>
@property (strong, nonatomic) UIWindow *window;
@end

@implementation WailsSceneDelegate
- (void)scene:(UIScene *)scene
    willConnectToSession:(UISceneSession *)session
            options:(UISceneConnectionOptions *)connectionOptions {
    if (![scene isKindOfClass:[UIWindowScene class]]) {
        return;
    }

    id applicationDelegate = UIApplication.sharedApplication.delegate;
    UIWindow *wailsWindow = [applicationDelegate valueForKey:@"window"];
    if (wailsWindow == nil) {
        wailsWindow = [[UIWindow alloc] initWithWindowScene:(UIWindowScene *)scene];
        wailsWindow.rootViewController = [[UIViewController alloc] init];
        [applicationDelegate setValue:wailsWindow forKey:@"window"];
    } else {
        wailsWindow.windowScene = (UIWindowScene *)scene;
    }
    self.window = wailsWindow;
    [self.window makeKeyAndVisible];
}
@end`)
	mainText = replaceOnce(mainText, "setvbuf(stderr, NULL, _IONBF, 0);", `setvbuf(stderr, NULL, _IONBF, 0);

        // Ensure iOS public Documents directory exists for the Files app
        NSArray *documentPaths = NSSearchPathForDirectoriesInDomains(NSDocumentDirectory, NSUserDomainMask, YES);
        if (documentPaths.count > 0) {
            NSString *docsDir = documentPaths.firstObject;
            NSFileManager *fm = [NSFileManager defaultManager];
            if (![fm fileExistsAtPath:docsDir]) {
                [fm createDirectoryAtPath:docsDir withIntermediateDirectories:YES attributes:nil error:nil];
            }
        }

        // Ensure translation bridge symbols remain reachable by the linker
        (void)DKSTAppleTranslationAvailable;
        (void)DKSTGoogleTranslationAvailable;

        dispatch_async(dispatch_get_main_queue(), ^{
            dkst_litertlm_server_start();
        });`)
	if err := os.WriteFile(mainPath, []byte(mainText), 0o644); err != nil {
		panic(err)
	}
	fmt.Println("Patched iOS Xcode project with LiteRT-LM (Embed & Sign + RPATH) & Native Translation Bridge")
}
