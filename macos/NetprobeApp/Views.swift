import SwiftUI
import Charts

enum SidebarItem: String, CaseIterable, Identifiable {
    case dashboard = "Dashboard"
    case incidents = "Incidents"
    case configuration = "Configuration"
    var id: String { rawValue }
    var icon: String {
        switch self { case .dashboard: "waveform.path.ecg"; case .incidents: "exclamationmark.triangle"; case .configuration: "slider.horizontal.3" }
    }
}

struct RootView: View {
    @EnvironmentObject var model: AppModel
    @State private var selection: SidebarItem? = .dashboard

    var body: some View {
        NavigationSplitView {
            List(SidebarItem.allCases, selection: $selection) { item in
                Label(item.rawValue, systemImage: item.icon).tag(item)
            }
            .navigationSplitViewColumnWidth(min: 170, ideal: 190)
        } detail: {
            Group {
                switch selection ?? .dashboard {
                case .dashboard: DashboardView()
                case .incidents: IncidentsView()
                case .configuration: ConfigurationView()
                }
            }
            .toolbar { MainToolbar() }
            .safeAreaInset(edge: .bottom) {
                HStack {
                    Circle().fill(model.isCollecting ? (model.failingTargets > 0 ? Color.orange : Color.green) : Color.secondary).frame(width: 8, height: 8)
                    Text(model.message).lineLimit(1)
                    Spacer()
                    Text(model.collectorSummary).foregroundStyle(.secondary)
                }
                .font(.caption).padding(.horizontal, 12).padding(.vertical, 7)
                .background(.bar)
            }
        }
        .sheet(isPresented: $model.showMarkerSheet) { MarkerSheet() }
    }
}

struct MainToolbar: ToolbarContent {
    @EnvironmentObject var model: AppModel
    var body: some ToolbarContent {
        ToolbarItemGroup {
            Picker("Range", selection: $model.selectedRange) {
                ForEach(AppModel.Range.allCases) { range in Text(range.label).tag(range) }
            }
            .frame(width: 165)
            .help("Changes the visible history. Collection continues until you press Stop.")
            .onChange(of: model.selectedRange) { _ in model.refresh() }
            Button { model.refresh() } label: { Label("Refresh", systemImage: "arrow.clockwise") }.disabled(model.isRefreshing)
            Button { model.showMarkerSheet = true } label: { Label("Mark", systemImage: "bookmark") }
            Button { model.exportReport() } label: { Label("Report", systemImage: "square.and.arrow.up") }
            if model.collectorOwned {
                Button { model.stopCollector() } label: { Label("Stop", systemImage: "stop.fill") }
            } else {
                Button { model.startCollector() } label: { Label("Start", systemImage: "play.fill") }
                    .disabled(model.isCollecting)
                    .help(model.isCollecting ? "A collector is already running outside this app" : "Start collecting network observations")
            }
        }
    }
}

struct DashboardView: View {
    @EnvironmentObject var model: AppModel
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                HStack(spacing: 12) {
                    MetricCard(title: "Collector", value: model.isCollecting ? "Running" : "Stopped", symbol: model.menuBarIcon)
                    MetricCard(title: "Targets", value: "\(model.targets.count)", symbol: "scope")
                    MetricCard(title: "Failing now", value: "\(model.failingTargets)", symbol: model.failingTargets > 0 ? "xmark.octagon.fill" : "checkmark.circle.fill")
                    MetricCard(title: "Incidents", value: "\(model.incidents.count)", symbol: "exclamationmark.triangle")
                }
                LatencyChart()
                    .frame(minHeight: 280)
                TargetList()
            }.padding(20)
        }.navigationTitle("Network health")
    }
}

struct MetricCard: View {
    let title: String; let value: String; let symbol: String
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(title, systemImage: symbol).font(.caption).foregroundStyle(.secondary)
            Text(value).font(.title2.bold())
        }.frame(maxWidth: .infinity, alignment: .leading).padding(14).background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 10))
    }
}

struct LatencyChart: View {
    @EnvironmentObject var model: AppModel
    @AppStorage("logarithmicYAxis") private var logarithmicYAxis = false
    struct ChartPoint: Identifiable {
        let observation: Observation
        let segment: String
        var id: Int64 { observation.id }
    }
    var filtered: [Observation] {
        guard let target = model.selectedTarget else { return model.observations }
        return model.observations.filter { $0.target == target }
    }
    var chartPoints: [ChartPoint] {
        let ordered = filtered.sorted { $0.timestamp < $1.timestamp }
        var segmentByTarget: [String: Int] = [:]
        var previousByTarget: [String: Date] = [:]
        return ordered.map { observation in
            let previous = previousByTarget[observation.target]
            if let previous, model.gaps.contains(where: { $0.start <= observation.timestamp && $0.end >= previous }) {
                segmentByTarget[observation.target, default: 0] += 1
            }
            previousByTarget[observation.target] = observation.timestamp
            return ChartPoint(
                observation: observation,
                segment: "\(observation.target)-\(segmentByTarget[observation.target, default: 0])"
            )
        }
    }
    var maxLatency: Double { max(10, filtered.compactMap(\.latencyMS).max() ?? 10) }
    var lossBaseline: Double {
        guard logarithmicYAxis else { return 0 }
        return max(0.1, (filtered.compactMap(\.latencyMS).filter { $0 > 0 }.min() ?? 0.1) * 0.8)
    }
    func chartLatency(_ latency: Double) -> Double {
        logarithmicYAxis ? max(0.1, latency) : latency
    }
    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("Latency and loss").font(.headline)
                Spacer()
                Toggle("Log scale", isOn: $logarithmicYAxis)
                    .toggleStyle(.checkbox)
                    .help("Use a logarithmic axis to keep large spikes from hiding smaller latency changes.")
                Picker("Target", selection: $model.selectedTarget) {
                    Text("All targets").tag(String?.none)
                    ForEach(model.targets) { Text($0.name).tag(Optional($0.name)) }
                }.frame(maxWidth: 240)
            }
            if filtered.isEmpty {
                EmptyState(title: "No observations", symbol: "waveform.path.ecg", detail: "Start the collector or select a longer time range.")
            } else {
                Chart {
                    ForEach(model.incidents) { incident in
                        RuleMark(x: .value("Incident", incident.start))
                            .foregroundStyle(.red.opacity(0.35)).lineStyle(StrokeStyle(lineWidth: 5))
                    }
                    ForEach(chartPoints.filter { $0.observation.success }) { point in
                        if let latency = point.observation.latencyMS {
                            LineMark(x: .value("Time", point.observation.timestamp), y: .value("Latency", chartLatency(latency)), series: .value("Run", point.segment))
                                .foregroundStyle(by: .value("Target", point.observation.target))
                                .interpolationMethod(.linear)
                        }
                    }
                    ForEach(filtered.filter { !$0.success }) { observation in
                        PointMark(x: .value("Time", observation.timestamp), y: .value("Failure", lossBaseline))
                            .foregroundStyle(.red).symbolSize(70)
                            .annotation(position: .top) { Image(systemName: "xmark").font(.caption2.bold()).foregroundStyle(.red) }
                    }
                    ForEach(model.markers) { marker in
                        RuleMark(x: .value("Marker", marker.timestamp)).foregroundStyle(.orange).lineStyle(StrokeStyle(dash: [3, 3]))
                    }
                }
                .chartYScale(type: logarithmicYAxis ? ScaleType.log : ScaleType.linear)
                .chartYAxisLabel("Milliseconds")
                .chartLegend(position: .bottom, alignment: .leading)
                .accessibilityLabel("Network latency and packet loss timeline")
            }
        }.padding(16).background(.background, in: RoundedRectangle(cornerRadius: 12)).overlay(RoundedRectangle(cornerRadius: 12).stroke(.separator.opacity(0.5)))
    }
}

struct TargetList: View {
    @EnvironmentObject var model: AppModel
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Targets").font(.headline)
            ForEach(model.targets, id: \.name) { target in
                TargetRow(target: target, latest: model.latestByTarget[target.name]) { model.selectedTarget = target.name }
                Divider()
            }
        }.padding(16).background(.background, in: RoundedRectangle(cornerRadius: 12)).overlay(RoundedRectangle(cornerRadius: 12).stroke(.separator.opacity(0.5)))
    }
}

struct IncidentsView: View {
    @EnvironmentObject var model: AppModel
    var body: some View {
        Group {
            if model.incidents.isEmpty {
                EmptyState(title: "No incidents", symbol: "checkmark.shield", detail: "No incidents were detected in this time range.")
            } else {
                List(model.incidents.reversed()) { incident in
                    VStack(alignment: .leading, spacing: 7) {
                        HStack {
                            Label(incident.scope, systemImage: "exclamationmark.triangle.fill").font(.headline)
                            Spacer()
                            Text(incident.start.formatted(date: .abbreviated, time: .standard)).foregroundStyle(.secondary)
                        }
                        Text(incident.explanation)
                        HStack {
                            Text(incident.affectedTargets.joined(separator: ", "))
                            Spacer()
                            Text("\(incident.packetLoss * 100, specifier: "%.1f")% event loss")
                        }.font(.caption).foregroundStyle(.secondary)
                    }.padding(.vertical, 7)
                }
            }
        }.navigationTitle("Likely incidents")
    }
}

struct ConfigurationView: View {
    @EnvironmentObject var model: AppModel
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Configuration").font(.title2.bold())
            TextField("Configuration path", text: $model.configPath).textFieldStyle(.roundedBorder)
            if model.configurationText.isEmpty {
                VStack(spacing: 12) {
                    EmptyState(title: "No configuration", symbol: "doc.badge.plus", detail: "Install the four default network targets to get started.")
                    Button("Install Defaults") { model.installExampleConfiguration() }
                }.frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                TextEditor(text: $model.configurationText).font(.system(.body, design: .monospaced)).scrollContentBackground(.hidden).padding(8).background(.quaternary.opacity(0.35), in: RoundedRectangle(cornerRadius: 8))
                HStack {
                    Button("Reload") { model.loadConfiguration() }
                    Spacer()
                    Button("Save and Validate") { model.saveAndValidateConfiguration() }.keyboardShortcut("s", modifiers: .command)
                }
            }
        }.padding(20).navigationTitle("Configuration")
    }
}

struct TargetRow: View {
    let target: ProbeTarget
    let latest: Observation?
    let select: () -> Void
    var statusSymbol: String { latest == nil ? "questionmark.circle" : latest!.success ? "checkmark.circle.fill" : "xmark.octagon.fill" }
    var statusColor: Color { guard let latest else { return .secondary }; return latest.success ? .green : .red }
    var detail: String { [target.probeType.uppercased(), latest?.interfaceName, latest?.route].compactMap { $0 }.joined(separator: " · ") }
    var result: String { latest?.latencyMS.map { String(format: "%.1f ms", $0) } ?? (latest?.errorCategory ?? "—") }
    var body: some View {
        Button(action: select) {
            HStack(spacing: 12) {
                Image(systemName: statusSymbol).foregroundStyle(statusColor)
                VStack(alignment: .leading) { Text(target.name).fontWeight(.medium); Text(detail).font(.caption).foregroundStyle(.secondary).lineLimit(1) }
                Spacer(); Text(result).monospacedDigit()
            }.contentShape(Rectangle()).padding(.vertical, 5)
        }.buttonStyle(.plain)
    }
}

struct EmptyState: View {
    let title: String; let symbol: String; let detail: String
    var body: some View {
        VStack(spacing: 10) {
            Image(systemName: symbol).font(.system(size: 32)).foregroundStyle(.secondary)
            Text(title).font(.headline)
            Text(detail).foregroundStyle(.secondary).multilineTextAlignment(.center)
        }.frame(maxWidth: .infinity, maxHeight: .infinity).padding(30)
    }
}

struct MarkerSheet: View {
    @EnvironmentObject var model: AppModel
    @Environment(\.dismiss) private var dismiss
    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Add network marker").font(.title2.bold())
            Text("Describe what you noticed. The marker will be correlated with nearby incidents.").foregroundStyle(.secondary)
            TextField("Video froze, VPN disconnected…", text: $model.markerText).textFieldStyle(.roundedBorder).onSubmit { model.addMarker() }
            HStack { Spacer(); Button("Cancel") { dismiss() }; Button("Add Marker") { model.addMarker() }.keyboardShortcut(.defaultAction).disabled(model.markerText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty) }
        }.padding(24).frame(width: 440)
    }
}
