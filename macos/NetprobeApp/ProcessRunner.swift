import Foundation

struct CommandResult: Sendable {
    let status: Int32
    let output: String
    let error: String
}

enum ProcessRunner {
    static func capture(executable: URL, arguments: [String]) async -> CommandResult {
        await Task.detached(priority: .utility) {
            let process = Process()
            let stdout = Pipe()
            let stderr = Pipe()
            process.executableURL = executable
            process.arguments = arguments
            process.standardOutput = stdout
            process.standardError = stderr
            do {
                try process.run()
                async let outputData = read(stdout.fileHandleForReading)
                async let errorData = read(stderr.fileHandleForReading)
                process.waitUntilExit()
                return CommandResult(
                    status: process.terminationStatus,
                    output: String(decoding: await outputData, as: UTF8.self),
                    error: String(decoding: await errorData, as: UTF8.self)
                )
            } catch {
                return CommandResult(status: -1, output: "", error: error.localizedDescription)
            }
        }.value
    }

    private static func read(_ handle: FileHandle) async -> Data {
        await Task.detached(priority: .utility) { handle.readDataToEndOfFile() }.value
    }
}

enum NetprobePaths {
    static var helper: URL? {
        if let override = ProcessInfo.processInfo.environment["NETPROBE_EXECUTABLE"], !override.isEmpty {
            return URL(fileURLWithPath: override)
        }
        let bundled = Bundle.main.bundleURL.appendingPathComponent("Contents/Helpers/netprobe")
        if FileManager.default.isExecutableFile(atPath: bundled.path) { return bundled }
        let candidates = ["/opt/homebrew/bin/netprobe", "/usr/local/bin/netprobe"]
        if let found = candidates.first(where: FileManager.default.isExecutableFile(atPath:)) {
            return URL(fileURLWithPath: found)
        }
        return nil
    }

    static var defaultConfig: URL {
        FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Application Support/netprobe/config.yaml")
    }

    static var exampleConfig: URL? {
        Bundle.main.url(forResource: "netprobe.example", withExtension: "yaml")
    }
}
