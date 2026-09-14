import SwiftUI
import AppKit

@MainActor
final class AppModel: ObservableObject {
    enum Range: String, CaseIterable, Identifiable {
        case fiveMinutes = "5m", fifteenMinutes = "15m", oneHour = "1h", sixHours = "6h"
        var id: String { rawValue }
        var label: String {
            switch self { case .fiveMinutes: "5 minutes"; case .fifteenMinutes: "15 minutes"; case .oneHour: "1 hour"; case .sixHours: "6 hours" }
        }
    }

    @Published var bundle: ExportBundle?
    @Published var selectedRange: Range = .fifteenMinutes
    @Published var selectedTarget: String?
    @Published var configurationText = ""
    @Published var configPath: String
    @Published var message = "Ready"
    @Published var isRefreshing = false
    @Published var collectorOwned = false
    @Published var showMarkerSheet = false
    @Published var markerText = ""

    private var collector: Process?
    private var pollingTask: Task<Void, Never>?
    private var actionMessageUntil = Date.distantPast

    init() {
        configPath = UserDefaults.standard.string(forKey: "configPath") ?? NetprobePaths.defaultConfig.path
        loadConfiguration()
    }

    var observations: [Observation] { bundle?.observations ?? [] }
    var incidents: [Incident] { bundle?.incidents ?? [] }
    var targets: [ProbeTarget] { bundle?.targets ?? [] }
    var latestByTarget: [String: Observation] {
        Dictionary(grouping: observations, by: \.target).compactMapValues { $0.max(by: { $0.timestamp < $1.timestamp }) }
    }
    var isCollecting: Bool {
        if collectorOwned { return true }
        guard let latest = observations.map(\.timestamp).max() else { return false }
        return Date().timeIntervalSince(latest) < 10
    }
    var failingTargets: Int { latestByTarget.values.filter { !$0.success }.count }
    var menuBarIcon: String { !isCollecting ? "network.slash" : failingTargets > 0 ? "exclamationmark.triangle.fill" : "network" }
    var collectorSummary: String {
        if !isCollecting { return "Collector stopped" }
        if failingTargets > 0 { return "\(failingTargets) target\(failingTargets == 1 ? "" : "s") failing" }
        return "All current targets healthy"
    }

    func startPolling() {
        guard pollingTask == nil else { return }
        pollingTask = Task { [weak self] in
            while !Task.isCancelled {
                self?.refresh()
                try? await Task.sleep(for: .seconds(2))
            }
        }
    }

    func refresh() {
        guard !isRefreshing, let helper = NetprobePaths.helper else {
            if NetprobePaths.helper == nil { message = "Bundled netprobe helper was not found" }
            return
        }
        isRefreshing = true
        let arguments = ["--config", configPath, "export", "--since", selectedRange.rawValue, "--format", "json"]
        Task {
            let result = await ProcessRunner.capture(executable: helper, arguments: arguments)
            defer { isRefreshing = false }
            guard result.status == 0 else { message = clean(result.error); return }
            do {
                bundle = try JSONDecoder.netprobe.decode(ExportBundle.self, from: Data(result.output.utf8))
                if selectedTarget == nil { selectedTarget = bundle?.targets.first?.name }
                if Date() >= actionMessageUntil {
                    message = "Updated \(Date().formatted(date: .omitted, time: .standard))"
                }
            } catch { message = "Could not read collector data: \(error.localizedDescription)" }
        }
    }

    func startCollector() {
        guard !isCollecting else { message = "A collector is already running"; return }
        guard let helper = NetprobePaths.helper else { message = "Bundled netprobe helper was not found"; return }
        do {
            let logURL = URL(fileURLWithPath: configPath).deletingLastPathComponent().appendingPathComponent("collector.log")
            try FileManager.default.createDirectory(at: logURL.deletingLastPathComponent(), withIntermediateDirectories: true)
            if !FileManager.default.fileExists(atPath: logURL.path) { FileManager.default.createFile(atPath: logURL.path, contents: nil) }
            let log = try FileHandle(forWritingTo: logURL); try log.seekToEnd()
            let process = Process(); process.executableURL = helper; process.arguments = ["--config", configPath, "run"]; process.standardOutput = log; process.standardError = log
            process.terminationHandler = { [weak self] process in Task { @MainActor in self?.collectorDidStop(status: process.terminationStatus); try? log.close() } }
            try process.run(); collector = process; collectorOwned = true; showActionMessage("Collector started")
        } catch { message = "Could not start collector: \(error.localizedDescription)" }
    }

    func stopCollector() {
        guard let collector, collector.isRunning else { collectorOwned = false; return }
        collector.terminate(); showActionMessage("Stopping collector…")
    }

    private func collectorDidStop(status: Int32) {
        collector = nil; collectorOwned = false
        showActionMessage(status == 0 || status == 15 ? "Collector stopped" : "Collector exited with status \(status)")
        refresh()
    }

    func addMarker() {
        let text = markerText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, let helper = NetprobePaths.helper else { return }
        markerText = ""; showMarkerSheet = false
        Task {
            let result = await ProcessRunner.capture(executable: helper, arguments: ["--config", configPath, "mark", text])
            showActionMessage(result.status == 0 ? "Marker recorded" : clean(result.error))
            refresh()
        }
    }

    func loadConfiguration() {
        configurationText = (try? String(contentsOfFile: configPath, encoding: .utf8)) ?? ""
    }

    func installExampleConfiguration() {
        guard let example = NetprobePaths.exampleConfig else { message = "Example configuration is missing"; return }
        do {
            let destination = URL(fileURLWithPath: configPath)
            try FileManager.default.createDirectory(at: destination.deletingLastPathComponent(), withIntermediateDirectories: true)
            if FileManager.default.fileExists(atPath: destination.path) { message = "A configuration already exists at this location"; return }
            try FileManager.default.copyItem(at: example, to: destination); loadConfiguration(); showActionMessage("Example installed—replace its sample hosts before collecting")
        } catch { message = "Could not install example: \(error.localizedDescription)" }
    }

    func saveAndValidateConfiguration() {
        do {
            let destination = URL(fileURLWithPath: configPath)
            try FileManager.default.createDirectory(at: destination.deletingLastPathComponent(), withIntermediateDirectories: true)
            try configurationText.write(to: destination, atomically: true, encoding: .utf8)
            UserDefaults.standard.set(configPath, forKey: "configPath")
        } catch { message = "Could not save configuration: \(error.localizedDescription)"; return }
        guard let helper = NetprobePaths.helper else { message = "Configuration saved; helper not found for validation"; return }
        Task {
            let result = await ProcessRunner.capture(executable: helper, arguments: ["--config", configPath, "config", "check"])
            showActionMessage(result.status == 0 ? result.output.trimmingCharacters(in: .whitespacesAndNewlines) : clean(result.error))
            if result.status == 0 { refresh() }
        }
    }

    func exportReport() {
        guard let helper = NetprobePaths.helper else { return }
        let panel = NSSavePanel(); panel.nameFieldStringValue = "netprobe-report.html"; panel.allowedContentTypes = [.html]
        guard panel.runModal() == .OK, let url = panel.url else { return }
        Task {
            let result = await ProcessRunner.capture(executable: helper, arguments: ["--config", configPath, "report", "--since", selectedRange.rawValue, "--format", "html", "--output", url.path])
            showActionMessage(result.status == 0 ? "Report saved to \(url.lastPathComponent)" : clean(result.error))
            if result.status == 0 { NSWorkspace.shared.activateFileViewerSelecting([url]) }
        }
    }

    private func clean(_ value: String) -> String {
        value.replacingOccurrences(of: "netprobe: ", with: "").trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func showActionMessage(_ value: String) {
        message = value
        actionMessageUntil = Date().addingTimeInterval(5)
    }
}
