import Foundation

struct ExportBundle: Decodable {
    var generatedAt: Date
    var from: Date
    var to: Date
    var targets: [ProbeTarget]
    var observations: [Observation]
    var incidents: [Incident]
    var markers: [Marker]
    var gaps: [CollectionGap]

    enum CodingKeys: String, CodingKey {
        case generatedAt = "generated_at", from, to, targets, observations, incidents, markers, gaps
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        generatedAt = try c.decode(Date.self, forKey: .generatedAt)
        from = try c.decode(Date.self, forKey: .from)
        to = try c.decode(Date.self, forKey: .to)
        targets = try c.decodeIfPresent([ProbeTarget].self, forKey: .targets) ?? []
        observations = try c.decodeIfPresent([Observation].self, forKey: .observations) ?? []
        incidents = try c.decodeIfPresent([Incident].self, forKey: .incidents) ?? []
        markers = try c.decodeIfPresent([Marker].self, forKey: .markers) ?? []
        gaps = try c.decodeIfPresent([CollectionGap].self, forKey: .gaps) ?? []
    }
}

struct ProbeTarget: Decodable, Identifiable, Hashable {
    var id: String { name }
    let name: String
    let description: String?
    let host: String?
    let probeType: String
    let port: Int?
    let resolver: String?
    let query: String?
    let url: String?
    let tags: [String]?

    enum CodingKeys: String, CodingKey {
        case name, description, host, port, resolver, query, url, tags
        case probeType = "probe_type"
    }
}

struct Observation: Decodable, Identifiable, Hashable {
    let id: Int64
    let timestamp: Date
    let target: String
    let probeType: String
    let success: Bool
    let latencyMS: Double?
    let errorCategory: String?
    let destination: String?
    let source: String?
    let interfaceName: String?
    let route: String?

    enum CodingKeys: String, CodingKey {
        case id, timestamp, target, success, route
        case probeType = "probe_type"
        case latencyMS = "latency_ms"
        case errorCategory = "error_category"
        case destination = "destination_address"
        case source = "source_address"
        case interfaceName = "interface"
    }
}

struct Incident: Decodable, Identifiable, Hashable {
    let id: Int64
    let start: Date
    let end: Date?
    let scope: String
    let explanation: String
    let affectedTargets: [String]
    let packetLoss: Double

    enum CodingKeys: String, CodingKey {
        case id, start, end, explanation
        case scope = "likely_scope"
        case affectedTargets = "affected_targets"
        case packetLoss = "packet_loss"
    }
}

struct Marker: Decodable, Identifiable, Hashable {
    let id: Int64
    let timestamp: Date
    let message: String
}

struct CollectionGap: Decodable, Identifiable, Hashable {
    let id: Int64
    let start: Date
    let end: Date
    let kind: String
    let details: String?
}

extension JSONDecoder {
    static var netprobe: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let value = try decoder.singleValueContainer().decode(String.self)
            let fractional = ISO8601DateFormatter()
            fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if let date = fractional.date(from: value) { return date }
            let ordinary = ISO8601DateFormatter()
            ordinary.formatOptions = [.withInternetDateTime]
            if let date = ordinary.date(from: value) { return date }
            throw DecodingError.dataCorruptedError(in: try decoder.singleValueContainer(), debugDescription: "Invalid RFC3339 date: \(value)")
        }
        return decoder
    }
}
