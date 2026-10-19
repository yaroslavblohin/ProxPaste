//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>
#import <Carbon/Carbon.h>
#import <QuartzCore/QuartzCore.h>
#include "_cgo_export.h"

static BOOL startHidden = NO;
static NSImage *brandIcon, *menuIcon;
static EventHotKeyRef activeKey = NULL, pendingKey = NULL;
static UInt32 activeID = 0, pendingID = 0, activeCode = 0, activeModifiers = 0, pendingCode = 0, pendingModifiers = 0;
int nativePrepareHotkey(unsigned int code, unsigned int modifiers) {
    if (activeKey && activeCode == code && activeModifiers == modifiers) return 1;
    pendingID = activeID == 1 ? 2 : 1;
    EventHotKeyID identity = {'PrxP', pendingID};
    OSStatus result = RegisterEventHotKey(code, modifiers, identity, GetApplicationEventTarget(), 0, &pendingKey);
    if (result != noErr) { pendingKey = NULL; return 0; }
    pendingCode = code; pendingModifiers = modifiers;
    return 1;
}
void nativeCommitHotkey(void) {
    if (!pendingKey) return;
    if (activeKey) UnregisterEventHotKey(activeKey);
    activeKey = pendingKey; pendingKey = NULL; activeID = pendingID;
    activeCode = pendingCode; activeModifiers = pendingModifiers;
}
void nativeCancelHotkey(void) { if (pendingKey) UnregisterEventHotKey(pendingKey); pendingKey = NULL; }
static NSRect F(CGFloat x, CGFloat y, CGFloat width, CGFloat height) { return NSMakeRect(x,730-y-height,width,height); }
static NSColor *ink(void) { return [NSColor colorWithSRGBRed:230/255.0 green:238/255.0 blue:247/255.0 alpha:1]; }
static NSColor *accent(void) { return [NSColor colorWithSRGBRed:14/255.0 green:139/255.0 blue:117/255.0 alpha:1]; }
static NSString *hotkeyTitle(void) { char *text = goHotkey(); NSString *title = [NSString stringWithUTF8String:text]; free(text); return title; }

int nativePermission(void) {
    NSDictionary *options = @{(__bridge NSString *)kAXTrustedCheckOptionPrompt: @YES};
    BOOL access = AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)options);
    BOOL listen = CGPreflightListenEventAccess();
    if (!listen) CGRequestListenEventAccess();
    return access && listen;
}
int nativeEscape(void) { return CGEventSourceKeyState(kCGEventSourceStateHIDSystemState, 53); }
int nativeModifiers(void) {
    CGEventFlags flags = CGEventSourceFlagsState(kCGEventSourceStateHIDSystemState);
    return (flags & (kCGEventFlagMaskShift | kCGEventFlagMaskControl | kCGEventFlagMaskAlternate | kCGEventFlagMaskCommand)) != 0;
}
int nativeKey(unsigned short code, int down, int shift) {
    CGEventRef event = CGEventCreateKeyboardEvent(NULL, code, down != 0);
    if (!event) return 0;
    CGEventSetFlags(event, shift ? kCGEventFlagMaskShift : 0);
    CGEventPost(kCGHIDEventTap, event);
    CFRelease(event);
    return 1;
}

@interface ProxApp : NSObject <NSApplicationDelegate, NSWindowDelegate>
@property(strong) NSWindow *window;
@property(strong) NSTextView *editor;
@property(strong) NSTextField *delay;
@property(strong) NSTextField *interval;
@property(strong) NSTextField *status;
@property(strong) NSButton *allow;
@property(strong) NSButton *load;
@property(strong) NSButton *start;
@property BOOL closing;
@property(strong) NSStatusItem *tray;
@property(strong) NSMenuItem *trayStatus;
@property EventHandlerRef hotkeyHandler;
@property BOOL hotkeyDown;
@property(strong) NSTextField *hotkeyField;
@property(strong) NSTextField *hotkeyHint;
@property(strong) NSButton *saveHotkey;
@property(strong) NSButton *resetHotkey;
@property(strong) NSMenuItem *trayHint;
- (void)pasteHotkey;
@end

static OSStatus handleHotkey(EventHandlerCallRef next, EventRef event, void *context) {
    ProxApp *app = (__bridge ProxApp *)context;
    EventHotKeyID key;
    if (GetEventParameter(event, kEventParamDirectObject, typeEventHotKeyID, NULL, sizeof(key), NULL, &key) != noErr || key.id != activeID || key.signature != 'PrxP') return eventNotHandledErr;
    if (GetEventKind(event) == kEventHotKeyReleased) { app.hotkeyDown = NO; return noErr; }
    if (!app.hotkeyDown) { app.hotkeyDown = YES; [app pasteHotkey]; }
    return noErr;
}

@implementation ProxApp
- (NSTextField *)label:(NSString *)text frame:(NSRect)frame {
    NSTextField *label = [NSTextField wrappingLabelWithString:text];
    label.frame = frame;
    label.font = [NSFont systemFontOfSize:14];
    label.textColor = [NSColor colorWithSRGBRed:155/255.0 green:173/255.0 blue:192/255.0 alpha:1];
    [self.window.contentView addSubview:label];
    return label;
}
- (NSButton *)button:(NSString *)text action:(SEL)action frame:(NSRect)frame {
    NSButton *button = [NSButton buttonWithTitle:text target:self action:action];
    button.frame = frame;
    button.font = [NSFont systemFontOfSize:14 weight:NSFontWeightSemibold];
    [self.window.contentView addSubview:button];
    return button;
}
- (void)card:(NSRect)frame {
    NSView *view = [[NSView alloc] initWithFrame:frame]; view.wantsLayer = YES;
    view.layer.backgroundColor = [NSColor colorWithSRGBRed:23/255.0 green:34/255.0 blue:48/255.0 alpha:1].CGColor;
    view.layer.cornerRadius = 12; view.layer.borderWidth = 1;
    view.layer.borderColor = [NSColor colorWithSRGBRed:45/255.0 green:61/255.0 blue:78/255.0 alpha:1].CGColor;
    [self.window.contentView addSubview:view];
}
- (void)section:(NSString *)text frame:(NSRect)frame {
    NSTextField *label = [self label:text frame:frame]; label.font = [NSFont systemFontOfSize:11 weight:NSFontWeightSemibold]; label.textColor = ink();
}
- (void)primary:(NSButton *)button {
    button.bordered = NO; button.wantsLayer = YES; button.layer.cornerRadius = 8;
    button.layer.backgroundColor = accent().CGColor;
    button.contentTintColor = NSColor.whiteColor;
}
- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    self.window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 840, 730)
        styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable
        backing:NSBackingStoreBuffered defer:NO];
    self.window.title = @"ProxPaste";
    self.window.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
    self.window.backgroundColor = [NSColor colorWithSRGBRed:15/255.0 green:23/255.0 blue:33/255.0 alpha:1];
    self.window.delegate = self;
    [self.window center];
    [self card:F(24,108,792,284)]; [self card:F(24,406,300,166)]; [self card:F(340,406,476,166)];
    NSImageView *logo = [[NSImageView alloc] initWithFrame:F(26,25,56,56)]; logo.image = brandIcon;
    [self.window.contentView addSubview:logo];
    NSTextField *title = [self label:@"ProxPaste" frame:F(104,22,420,40)];
    title.font = [NSFont boldSystemFontOfSize:30]; title.textColor = ink();
    [self label:@"Ваш буфер. Ввод с клавиатуры. Любая консоль." frame:F(106,66,650,24)];
    [self section:@"ТЕКСТ ДЛЯ ВВОДА" frame:F(44,128,350,24)];
    self.load = [self button:@"Из буфера обмена" action:@selector(loadClipboard:) frame:F(588,119,210,36)];
    NSScrollView *scroll = [[NSScrollView alloc] initWithFrame:F(44,168,752,202)];
    scroll.borderType = NSNoBorder;
    scroll.hasVerticalScroller = YES;
    self.editor = [[NSTextView alloc] initWithFrame:NSMakeRect(0, 0, 730, 202)];
    self.editor.richText = NO;
    self.editor.font = [NSFont monospacedSystemFontOfSize:14 weight:NSFontWeightRegular];
    self.editor.backgroundColor = [NSColor colorWithSRGBRed:10/255.0 green:17/255.0 blue:26/255.0 alpha:1];
    self.editor.textColor = [NSColor colorWithSRGBRed:226/255.0 green:237/255.0 blue:245/255.0 alpha:1];
    self.editor.insertionPointColor = NSColor.whiteColor;
    self.editor.textContainerInset = NSMakeSize(12,12);
    self.editor.automaticQuoteSubstitutionEnabled = NO;
    self.editor.automaticDashSubstitutionEnabled = NO;
    self.editor.automaticTextReplacementEnabled = NO;
    self.editor.automaticSpellingCorrectionEnabled = NO;
    self.editor.verticallyResizable = YES;
    self.editor.autoresizingMask = NSViewWidthSizable;
    self.editor.textContainer.widthTracksTextView = YES;
    scroll.documentView = self.editor;
    [self.window.contentView addSubview:scroll];
    [self section:@"ТЕМП ВВОДА" frame:F(44,426,260,22)];
    [self label:@"До старта, сек" frame:F(44,467,164,24)];
    self.delay = [[NSTextField alloc] initWithFrame:F(226,462,72,30)];
    self.delay.integerValue = goDelaySeconds();
    [self.window.contentView addSubview:self.delay];
    [self label:@"Интервал, мс" frame:F(44,517,164,24)];
    self.interval = [[NSTextField alloc] initWithFrame:F(226,512,72,30)];
    self.interval.integerValue = goIntervalMS();
    [self.window.contentView addSubview:self.interval];
    [self section:@"ГОРЯЧАЯ КЛАВИША" frame:F(360,426,400,22)];
    self.hotkeyField = [[NSTextField alloc] initWithFrame:F(360,462,256,32)];
    self.hotkeyField.stringValue = hotkeyTitle(); self.hotkeyField.font = [NSFont systemFontOfSize:15];
    [self.window.contentView addSubview:self.hotkeyField];
    self.saveHotkey = [self button:@"Сохранить всё" action:@selector(saveShortcut:) frame:F(632,462,164,32)];
    [self primary:self.saveHotkey];
    self.hotkeyHint = [self label:@"" frame:F(360,507,258,22)]; self.hotkeyHint.font = [NSFont systemFontOfSize:12];
    NSTextField *example = [self label:@"Ctrl / Option / Shift / Command + A–Z, 0–9, F1–F12" frame:F(360,540,438,22)]; example.font = [NSFont systemFontOfSize:10];
    self.resetHotkey = [self button:@"По умолчанию" action:@selector(resetShortcut:) frame:F(630,504,168,32)];
    self.allow = [NSButton checkboxWithTitle:@"Разрешить Enter и Tab — Enter может выполнить команду" target:nil action:nil];
    self.allow.frame = F(26,586,790,28);
    self.allow.state = goAllowEnterTab() ? NSControlStateValueOn : NSControlStateValueOff;
    [self.window.contentView addSubview:self.allow];
    self.start = [self button:@"Начать ввод" action:@selector(begin:) frame:F(24,630,226,42)]; [self primary:self.start];
    [self button:@"Остановить · Esc" action:@selector(cancel:) frame:F(264,630,208,42)];
    NSTextField *layout = [self label:@"English (US) · Caps Lock выключен" frame:F(496,633,320,22)]; layout.font = [NSFont systemFontOfSize:12];
    NSTextField *hintText = [self label:@"Закрытие окна — в строку меню" frame:F(496,655,320,22)]; hintText.font = [NSFont systemFontOfSize:11];
    self.status = [self label:@"Готово." frame:F(26,688,788,36)]; self.status.font = [NSFont systemFontOfSize:12];
    self.tray = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
    self.tray.button.image = menuIcon;
    if (!self.tray.button.image) self.tray.button.title = @"PP";
    self.tray.button.toolTip = [@"ProxPaste · " stringByAppendingString:hotkeyTitle()];
    NSMenu *trayMenu = [[NSMenu alloc] initWithTitle:@"ProxPaste"];
    self.trayStatus = [trayMenu addItemWithTitle:@"Готово" action:nil keyEquivalent:@""];
    self.trayStatus.enabled = NO;
    [trayMenu addItem:[NSMenuItem separatorItem]];
    NSMenuItem *show = [trayMenu addItemWithTitle:@"Открыть ProxPaste" action:@selector(showWindow:) keyEquivalent:@""];
    show.target = self;
    self.trayHint = [trayMenu addItemWithTitle:@"" action:nil keyEquivalent:@""];
    self.trayHint.enabled = NO;
    NSMenuItem *cancel = [trayMenu addItemWithTitle:@"Остановить ввод" action:@selector(cancel:) keyEquivalent:@""];
    cancel.target = self;
    [trayMenu addItem:[NSMenuItem separatorItem]];
    [trayMenu addItemWithTitle:@"Выход" action:@selector(terminate:) keyEquivalent:@""];
    self.tray.menu = trayMenu;
    EventTypeSpec events[] = {{kEventClassKeyboard, kEventHotKeyPressed}, {kEventClassKeyboard, kEventHotKeyReleased}};
    EventHandlerRef handler = NULL;
    OSStatus installed = InstallApplicationEventHandler(&handleHotkey, 2, events, (__bridge void *)self, &handler);
    self.hotkeyHandler = handler;
    if (installed == noErr) goInstallHotkey();
    else goSetStatus("Не удалось включить обработчик горячей клавиши.");
    [self updateShortcutHint];
    [NSTimer scheduledTimerWithTimeInterval:0.1 target:self selector:@selector(tick:) userInfo:nil repeats:YES];
    if (!startHidden) {
        [self.window makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
    }
}
- (void)updateShortcutHint {
    self.hotkeyHint.stringValue = activeKey ? [@"Активно: " stringByAppendingString:hotkeyTitle()] : @"Сочетание не зарегистрировано";
    self.trayHint.title = [@"Вставка: " stringByAppendingString:hotkeyTitle()];
}
- (void)saveShortcut:(id)sender {
    if (goChangeSettings((char *)self.hotkeyField.stringValue.UTF8String, (char *)self.delay.stringValue.UTF8String,
        (char *)self.interval.stringValue.UTF8String, self.allow.state == NSControlStateValueOn)) {
        self.hotkeyField.stringValue = hotkeyTitle(); self.hotkeyDown = NO; [self updateShortcutHint];
    }
    [self tick:nil];
}
- (void)resetShortcut:(id)sender {
    if (goBusy()) return;
    self.hotkeyField.stringValue = @"Ctrl+Option+V"; [self saveShortcut:nil];
}
- (void)loadClipboard:(id)sender {
    NSString *text = [[NSPasteboard generalPasteboard] stringForType:NSPasteboardTypeString];
    if (text) self.editor.string = text;
    else goSetStatus("В буфере нет текста.");
}
- (void)showWindow:(id)sender {
    [self.window makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
}
- (void)pasteHotkey {
    if (goBusy() || self.closing) return;
    NSString *text = [[NSPasteboard generalPasteboard] stringForType:NSPasteboardTypeString];
    if (!text) { goSetStatus("В буфере нет текста."); NSBeep(); return; }
    self.editor.string = text;
    goStart((char *)text.UTF8String, (char *)self.delay.stringValue.UTF8String,
        (char *)self.interval.stringValue.UTF8String, self.allow.state == NSControlStateValueOn);
    if (!goBusy()) NSBeep();
    [self tick:nil];
}
- (void)begin:(id)sender {
    goStart((char *)self.editor.string.UTF8String, (char *)self.delay.stringValue.UTF8String,
        (char *)self.interval.stringValue.UTF8String, self.allow.state == NSControlStateValueOn);
    [self tick:nil];
}
- (void)cancel:(id)sender { goStop(); }
- (void)tick:(id)sender {
    BOOL busy = goBusy() != 0;
    self.editor.editable = !busy;
    self.load.enabled = !busy;
    self.start.enabled = !busy;
    self.delay.enabled = !busy;
    self.interval.enabled = !busy;
    self.allow.enabled = !busy;
    self.hotkeyField.enabled = !busy; self.saveHotkey.enabled = !busy; self.resetHotkey.enabled = !busy;
    char *status = goStatus();
    self.status.stringValue = [NSString stringWithUTF8String:status];
    self.trayStatus.title = self.status.stringValue;
    self.tray.button.toolTip = [NSString stringWithFormat:@"ProxPaste · %@\n%@",hotkeyTitle(),self.status.stringValue];
    free(status);
    if (self.closing && !busy) [NSApp terminate:nil];
}
- (BOOL)windowShouldClose:(NSWindow *)sender {
    [sender orderOut:nil];
    return NO;
}
- (BOOL)applicationShouldHandleReopen:(NSApplication *)sender hasVisibleWindows:(BOOL)visible {
    [self showWindow:nil];
    return YES;
}
- (void)applicationWillTerminate:(NSNotification *)notification {
    if (activeKey) UnregisterEventHotKey(activeKey);
    nativeCancelHotkey();
    if (self.hotkeyHandler) RemoveEventHandler(self.hotkeyHandler);
    [[NSStatusBar systemStatusBar] removeStatusItem:self.tray];
}
- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender {
    if (goBusy()) { self.closing = YES; goStop(); return NSTerminateCancel; }
    return NSTerminateNow;
}
@end

void nativeRun(int hidden, const void *icon, int iconLength, const void *tray, int trayLength) {
    startHidden = hidden != 0;
    @autoreleasepool {
        [NSApplication sharedApplication];
        brandIcon = [[NSImage alloc] initWithData:[NSData dataWithBytes:icon length:iconLength]];
        menuIcon = [[NSImage alloc] initWithData:[NSData dataWithBytes:tray length:trayLength]];
        menuIcon.size = NSMakeSize(20,20); menuIcon.template = YES;
        [NSApp setApplicationIconImage:brandIcon];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        NSMenu *menu = [[NSMenu alloc] init];
        NSMenuItem *appItem = [[NSMenuItem alloc] init];
        [menu addItem:appItem];
        NSMenu *appMenu = [[NSMenu alloc] initWithTitle:@"ProxPaste"];
        [appMenu addItemWithTitle:@"Quit ProxPaste" action:@selector(terminate:) keyEquivalent:@"q"];
        appItem.submenu = appMenu;
        NSMenuItem *editItem = [[NSMenuItem alloc] init];
        [menu addItem:editItem];
        NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
        [edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
        [edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
        [edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
        [edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
        editItem.submenu = edit;
        NSApp.mainMenu = menu;
        ProxApp *delegate = [[ProxApp alloc] init];
        NSApp.delegate = delegate;
        [NSApp run];
    }
}
