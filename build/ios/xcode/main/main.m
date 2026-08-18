//go:build ios
// Minimal bootstrap: delegate comes from Go archive (WailsAppDelegate)
#import <UIKit/UIKit.h>
#include <stdio.h>
#include <stdint.h>
#include <dispatch/dispatch.h>
extern void dkst_litertlm_server_start(void);

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
@end
extern int32_t DKSTAppleTranslationAvailable(void);
extern int32_t DKSTGoogleTranslationAvailable(void);

int main(int argc, char * argv[]) {
    @autoreleasepool {
        // Disable buffering so stdout/stderr from Go log.Printf flush immediately
        setvbuf(stdout, NULL, _IONBF, 0);
        setvbuf(stderr, NULL, _IONBF, 0);

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
        });

        // Call UIApplicationMain IMMEDIATELY and start NOTHING else here. Do not
        // start the Go runtime yet: starting it concurrently with UIApplicationMain
        // intermittently corrupts the FrontBoard launch handshake on a physical
        // device, so the app delegate's didFinishLaunchingWithOptions never fires
        // (blank cold launch / 0x8BADF00D). Instead, the WailsAppDelegate (provided
        // by the Go archive) starts the Go runtime itself from
        // didFinishLaunchingWithOptions — i.e. only AFTER UIKit has delivered the
        // launch — so the runtime never races the launch handshake.
        return UIApplicationMain(argc, argv, nil, @"WailsAppDelegate");
    }
}
