//go:build darwin

#import <Cocoa/Cocoa.h>

@interface LiveToolTitlebarBackgroundView : NSView
@end

@implementation LiveToolTitlebarBackgroundView
- (NSView *)hitTest:(NSPoint)point {
	return nil;
}
@end

static NSView *titlebarContainer(NSWindow *window) {
	NSButton *closeButton = [window standardWindowButton:NSWindowCloseButton];
	NSView *contentView = [window contentView];
	NSView *contentSuperview = [contentView superview];
	if (closeButton == nil || contentSuperview == nil) {
		return nil;
	}

	CGFloat windowWidth = NSWidth([contentSuperview bounds]);
	for (NSView *view = [closeButton superview]; view != nil && view != contentSuperview; view = [view superview]) {
		CGFloat height = NSHeight([view bounds]);
		CGFloat width = NSWidth([view bounds]);
		if (height >= 22 && height <= 80 && width >= windowWidth - 2) {
			return view;
		}
	}
	return nil;
}

static void applyOpaqueTitlebar(NSWindow *window) {
	if (window == nil) {
		return;
	}

	NSView *container = titlebarContainer(window);
	if (container == nil) {
		return;
	}

	for (NSView *subview in [container subviews]) {
		if ([[subview identifier] isEqualToString:@"livetool.opaque-titlebar-background"]) {
			return;
		}
	}

	LiveToolTitlebarBackgroundView *background = [[LiveToolTitlebarBackgroundView alloc] initWithFrame:[container bounds]];
	[background setIdentifier:@"livetool.opaque-titlebar-background"];
	[background setWantsLayer:YES];
	[[background layer] setBackgroundColor:[[NSColor colorWithSRGBRed:0.957 green:0.965 blue:0.980 alpha:1.0] CGColor]];
	[background setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
	[container addSubview:background positioned:NSWindowBelow relativeTo:nil];
}

void livetoolSetOpaqueMacTitlebar(void *windowPointer) {
	NSWindow *window = (NSWindow *)windowPointer;
	if (window == nil) {
		return;
	}
	dispatch_async(dispatch_get_main_queue(), ^{
		applyOpaqueTitlebar(window);
	});
}
