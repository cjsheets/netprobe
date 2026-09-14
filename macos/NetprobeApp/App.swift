import SwiftUI
import AppKit

@main
struct NetprobeMacApp: App {
    @StateObject private var model = AppModel()
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate

    var body: some Scene {
        WindowGroup("Netprobe", id: "main") {
            RootView()
                .environmentObject(model)
                .frame(minWidth: 820, minHeight: 560)
                .onAppear {
                    delegate.model = model
                    model.startPolling()
                }
        }
        .commands {
            CommandGroup(after: .newItem) {
                Button("Refresh") { model.refresh() }
                    .keyboardShortcut("r", modifiers: .command)
                Button("Add Marker…") { model.showMarkerSheet = true }
                    .keyboardShortcut("m", modifiers: [.command, .shift])
            }
        }

        MenuBarExtra("Netprobe", systemImage: model.menuBarIcon) {
            Text(model.collectorSummary)
            Divider()
            if model.collectorOwned {
                Button("Stop Collector") { model.stopCollector() }
            } else if model.isCollecting {
                Text("Collector managed outside this app")
            } else {
                Button("Start Collector") { model.startCollector() }
            }
            Button("Add Marker…") { model.showMarkerSheet = true }
            Button("Refresh") { model.refresh() }
            Divider()
            Button("Quit Netprobe") { NSApplication.shared.terminate(nil) }
        }
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    var model: AppModel?
    func applicationWillTerminate(_ notification: Notification) {
        model?.stopCollector()
    }
}
