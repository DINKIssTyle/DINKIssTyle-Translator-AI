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
		panic("LiteRT-LM Xcode patch anchor not found: " + old)
	}
	return strings.Replace(text, old, replacement, 1)
}

func main() {
	projectPath := filepath.Join("build", "ios", "xcode", "main.xcodeproj", "project.pbxproj")
	infoPlistPath := filepath.Join("build", "ios", "xcode", "main", "Info.plist")
	swiftDestination := filepath.Join("build", "ios", "xcode", "main", "LiteRTLMServer.swift")
	swiftSource := filepath.Join("build", "ios", "LiteRTLMServer.swift")

	swiftData, err := os.ReadFile(swiftSource)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(swiftDestination, swiftData, 0o644); err != nil {
		panic(err)
	}

	// Wails 3 currently emits CFBundleExecutable=wailsapp even though the
	// generated Xcode target names its executable after PRODUCT_NAME. iOS then
	// rejects the otherwise valid bundle as "missing its bundle executable".
	// Use Xcode's canonical build setting so this remains correct if the product
	// name changes later.
	infoPlistData, err := os.ReadFile(infoPlistPath)
	if err != nil {
		panic(err)
	}
	infoPlist := string(infoPlistData)
	infoPlist = replaceOnce(infoPlist,
		"<string>wailsapp</string>",
		"<string>$(EXECUTABLE_NAME)</string>")
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
	if err := os.WriteFile(infoPlistPath, []byte(infoPlist), 0o644); err != nil {
		panic(err)
	}

	raw, err := os.ReadFile(projectPath)
	if err != nil {
		panic(err)
	}
	text := string(raw)
	text = replaceOnce(text,
		"C0DEBEEF0000000000000001 /* main.m in Sources */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000002 /* main.m */; };",
		"C0DEBEEF0000000000000001 /* main.m in Sources */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000002 /* main.m */; };\n\t\tC0DEBEEF0000000000000201 /* LiteRTLMServer.swift in Sources */ = {isa = PBXBuildFile; fileRef = C0DEBEEF0000000000000202 /* LiteRTLMServer.swift */; };\n\t\tC0DEBEEF0000000000000203 /* LiteRTLMNative in Frameworks */ = {isa = PBXBuildFile; productRef = C0DEBEEF0000000000000204 /* LiteRTLMNative */; };")
	text = replaceOnce(text,
		"C0DEBEEF0000000000000002 /* main.m */ = {isa = PBXFileReference; lastKnownFileType = sourcecode.c.objc; path = main.m; sourceTree = \"<group>\"; };",
		"C0DEBEEF0000000000000002 /* main.m */ = {isa = PBXFileReference; lastKnownFileType = sourcecode.c.objc; path = main.m; sourceTree = \"<group>\"; };\n\t\tC0DEBEEF0000000000000202 /* LiteRTLMServer.swift */ = {isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = LiteRTLMServer.swift; sourceTree = \"<group>\"; };")
	text = replaceOnce(text,
		"C0DEBEEF0000000000000002 /* main.m */,\n\t\t\t\tC0DEBEEF0000000000000003 /* Info.plist */,",
		"C0DEBEEF0000000000000002 /* main.m */,\n\t\t\t\tC0DEBEEF0000000000000202 /* LiteRTLMServer.swift */,\n\t\t\t\tC0DEBEEF0000000000000003 /* Info.plist */,")
	text = replaceOnce(text,
		"C0DEBEEF00000000000000F7 /* DKST Translator AI.a in Frameworks */,",
		"C0DEBEEF00000000000000F7 /* DKST Translator AI.a in Frameworks */,\n\t\t\t\tC0DEBEEF0000000000000203 /* LiteRTLMNative in Frameworks */,")
	text = replaceOnce(text,
		"C0DEBEEF0000000000000001 /* main.m in Sources */,",
		"C0DEBEEF0000000000000001 /* main.m in Sources */,\n\t\t\t\tC0DEBEEF0000000000000201 /* LiteRTLMServer.swift in Sources */,")
	text = replaceOnce(text,
		"productReference = C0DEBEEF0000000000000004 /* DKST Translator AI.app */;",
		"packageProductDependencies = (\n\t\t\t\tC0DEBEEF0000000000000204 /* LiteRTLMNative */,\n\t\t\t);\n\t\t\tproductReference = C0DEBEEF0000000000000004 /* DKST Translator AI.app */;")
	text = replaceOnce(text,
		"projectRoot = \"\";\n\t\t\ttargets = (",
		"projectRoot = \"\";\n\t\t\tpackageReferences = (\n\t\t\t\tC0DEBEEF0000000000000205 /* XCLocalSwiftPackageReference LiteRTLMNative */,\n\t\t\t);\n\t\t\ttargets = (")
	text = replaceOnce(text,
		"/* Begin XCBuildConfiguration section */",
		"/* Begin XCLocalSwiftPackageReference section */\n\t\tC0DEBEEF0000000000000205 /* XCLocalSwiftPackageReference LiteRTLMNative */ = {isa = XCLocalSwiftPackageReference; relativePath = ../LiteRTLMNative; };\n/* End XCLocalSwiftPackageReference section */\n\n/* Begin XCSwiftPackageProductDependency section */\n\t\tC0DEBEEF0000000000000204 /* LiteRTLMNative */ = {isa = XCSwiftPackageProductDependency; package = C0DEBEEF0000000000000205 /* XCLocalSwiftPackageReference LiteRTLMNative */; productName = LiteRTLMNative; };\n/* End XCSwiftPackageProductDependency section */\n\n/* Begin XCBuildConfiguration section */")
	text = replaceOnce(text,
		`export CGO_LDFLAGS=\"-isysroot ${SDK_PATH} -target ${GO_TARGET}\"\ncd \"${APP_ROOT}\"`,
		`export CGO_LDFLAGS=\"-isysroot ${SDK_PATH} -target ${GO_TARGET}\"\n# Xcode does not inherit the interactive shell PATH. Locate toolchains explicitly.\nexport PATH=\"/usr/local/go/bin:/opt/homebrew/bin:${HOME}/go/bin:${HOME}/.local/bin:${PATH}\"\nGO_BIN=\"${GO_BINARY:-}\"\nif [ -z \"${GO_BIN}\" ]; then GO_BIN=$(command -v go 2>/dev/null || true); fi\nif [ -z \"${GO_BIN}\" ] || [ ! -x \"${GO_BIN}\" ]; then\n  echo \"Go was not found. Install Go or set GO_BINARY to its absolute path in the Xcode scheme.\" >&2\n  exit 127\nfi\ncd \"${APP_ROOT}\"`)
	text = replaceOnce(text,
		`if [ ! -f build/ios/xcode/overlay.json ]; then\n  wails3 ios overlay:gen -out build/ios/xcode/overlay.json -config build/config.yml || true\nfi`,
		`if [ ! -f build/ios/xcode/overlay.json ]; then\n  WAILS_BIN=\"${WAILS3_BINARY:-}\"\n  if [ -z \"${WAILS_BIN}\" ]; then WAILS_BIN=$(command -v wails3 2>/dev/null || true); fi\n  if [ -z \"${WAILS_BIN}\" ] || [ ! -x \"${WAILS_BIN}\" ]; then\n    echo \"wails3 was not found. Install it or set WAILS3_BINARY to its absolute path in the Xcode scheme.\" >&2\n    exit 127\n  fi\n  \"${WAILS_BIN}\" ios overlay:gen -out build/ios/xcode/overlay.json -config build/config.yml\nfi`)
	text = replaceOnce(text,
		`go build -buildmode=c-archive -overlay build/ios/xcode/overlay.json -o \"bin/DKST Translator AI.a\"`,
		`\"${GO_BIN}\" build -buildmode=c-archive -overlay build/ios/xcode/overlay.json -o \"bin/DKST Translator AI.a\"`)
	if !strings.Contains(text, "SWIFT_VERSION = 5.0;") {
		text = strings.ReplaceAll(text,
			"INFOPLIST_FILE = main/Info.plist;",
			"INFOPLIST_FILE = main/Info.plist;\n\t\t\t\tSWIFT_VERSION = 5.0;\n\t\t\t\t\"EXCLUDED_ARCHS[sdk=iphonesimulator*]\" = x86_64;")
	}
	if !strings.Contains(text, "LD_RUNPATH_SEARCH_PATHS = (") {
		text = strings.ReplaceAll(text,
			"INFOPLIST_FILE = main/Info.plist;",
			"INFOPLIST_FILE = main/Info.plist;\n\t\t\t\tALWAYS_SEARCH_USER_PATHS = NO;\n\t\t\t\tLD_RUNPATH_SEARCH_PATHS = (\n\t\t\t\t\t\"$(inherited)\",\n\t\t\t\t\t\"@executable_path/Frameworks\",\n\t\t\t\t);")
	}
	// Keep simulator builds unsigned, but allow Xcode automatic signing for a
	// connected iPhone. The generated project disables signing for every SDK.
	text = strings.ReplaceAll(text,
		"CODE_SIGNING_ALLOWED = NO;",
		"CODE_SIGN_STYLE = Automatic;\n\t\t\t\t\"CODE_SIGNING_ALLOWED[sdk=iphonesimulator*]\" = NO;")
	if err := os.WriteFile(projectPath, []byte(text), 0o644); err != nil {
		panic(err)
	}

	mainPath := filepath.Join("build", "ios", "xcode", "main", "main.m")
	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		panic(err)
	}
	mainText := string(mainData)
	mainText = replaceOnce(mainText, "#include <stdio.h>", "#include <stdio.h>\n#include <dispatch/dispatch.h>\nextern void dkst_litertlm_server_start(void);")
	mainText = replaceOnce(mainText,
		"extern void dkst_litertlm_server_start(void);",
		`extern void dkst_litertlm_server_start(void);

// iOS 27 requires scene lifecycle adoption. Wails still owns and creates the
// application window; this delegate attaches that window to UIKit's scene.
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
	mainText = replaceOnce(mainText, "setvbuf(stderr, NULL, _IONBF, 0);", "setvbuf(stderr, NULL, _IONBF, 0);\n\n        dispatch_async(dispatch_get_main_queue(), ^{\n            dkst_litertlm_server_start();\n        });")
	if err := os.WriteFile(mainPath, []byte(mainText), 0o644); err != nil {
		panic(err)
	}
	fmt.Println("Patched iOS Xcode project with the LiteRT-LM 0.14.0 native package")
}
