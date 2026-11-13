import Foundation

struct InstallerFailure: LocalizedError {
    let message: String
    let details: String?
    var errorDescription: String? { message }
    init(_ message: String, details: String? = nil) { self.message = message; self.details = details }
}

struct InstallerBundle {
    let version: String
    let build: String

    static func inspect(_ app: URL, expectedIdentifier: String = "com.cassette.proxycat") throws -> InstallerBundle {
        let attributes = try FileManager.default.attributesOfItem(atPath: app.path)
        guard attributes[.type] as? FileAttributeType == .typeDirectory,
              let data = FileManager.default.contents(atPath: app.appendingPathComponent("Contents/Info.plist").path),
              let info = try PropertyListSerialization.propertyList(from: data, format: nil) as? [String: Any],
              info["CFBundleIdentifier"] as? String == expectedIdentifier,
              let version = info["CFBundleShortVersionString"] as? String,
              let build = info["CFBundleVersion"] as? String else {
            throw InstallerFailure("没有找到有效的 ProxyCat 应用。请从完整的安装磁盘打开安装程序。")
        }
        return InstallerBundle(version: version, build: build)
    }
}

enum InstallerCommand {
    @discardableResult
    static func run(_ executable: String, _ arguments: [String]) throws -> String {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: executable)
        process.arguments = arguments
        let output = Pipe()
        process.standardOutput = output
        process.standardError = output
        try process.run()
        // Drain before waiting: a command must not block on a full pipe.
        let data = output.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        let text = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
        guard process.terminationStatus == 0 else {
            throw InstallerFailure(text.isEmpty ? "安装操作未完成（\(process.terminationStatus)）。" : text)
        }
        return text
    }

    static func verifySignature(_ app: URL) throws {
        do { try run("/usr/bin/codesign", ["--verify", "--deep", "--strict", app.path]) }
        catch {
            let detail = error.localizedDescription
            let message = detail.contains("FinderInfo") || detail.contains("resource fork")
                ? "安装包带有不兼容的文件元数据，请使用修正版安装包。旧版未被替换。"
                : "应用完整性校验失败，旧版未被替换。请重新获取安装包。"
            throw InstallerFailure(message, details: detail)
        }
    }

    static func ensureProxyCatStopped() throws {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/pgrep")
        process.arguments = ["-x", "ProxyCat"]
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        try process.run()
        process.waitUntilExit()
        guard process.terminationStatus == 1 else {
            throw InstallerFailure(process.terminationStatus == 0
                ? "ProxyCat 仍在运行，尚未替换。请等待它安全退出后重试。"
                : "无法确认 ProxyCat 已退出，尚未替换旧版。")
        }
    }

    static func shellQuote(_ value: String) -> String { "'" + value.replacingOccurrences(of: "'", with: "'\\''") + "'" }
    static func appleScriptQuote(_ value: String) -> String {
        "\"" + value.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "\"", with: "\\\"") + "\""
    }
}

// All paths are supplied by the native installer, never by the subscription.
// Tests use temporary directories and injected validation/process checks.
struct InstallerTransaction {
    var files = FileManager.default
    var verify: (URL) throws -> Void = InstallerCommand.verifySignature
    var ensureStopped: () throws -> Void = InstallerCommand.ensureProxyCatStopped
    var prepareOwnership: (URL) throws -> Void = { _ in }
    var move: (URL, URL) throws -> Void = { try FileManager.default.moveItem(at: $0, to: $1) }

    func install(source: URL, destination: URL) throws {
        guard source.standardizedFileURL != destination.standardizedFileURL else {
            throw InstallerFailure("安装源与目标相同，未进行替换。")
        }
        let expected = try InstallerBundle.inspect(source)
        try verify(source)
        let parent = destination.deletingLastPathComponent()
        try files.createDirectory(at: parent, withIntermediateDirectories: true)
        let staging = parent.appendingPathComponent(".ProxyCat-install-\(UUID().uuidString)", isDirectory: true)
        let candidate = staging.appendingPathComponent("ProxyCat.app", isDirectory: true)
        let backup = parent.appendingPathComponent(".ProxyCat-backup-\(UUID().uuidString).app", isDirectory: true)
        try files.createDirectory(at: staging, withIntermediateDirectories: false)
        defer { try? files.removeItem(at: staging) }
        try files.copyItem(at: source, to: candidate)
        try prepareOwnership(candidate)
        try verify(candidate)
        let staged = try InstallerBundle.inspect(candidate)
        guard staged.version == expected.version, staged.build == expected.build else {
            throw InstallerFailure("复制后的应用版本不一致，未替换旧版。")
        }
        // Recheck after copying/authorization, not only before asking to quit.
        try ensureStopped()
        var backedUp = false
        var installed = false
        do {
            // Lstat also rejects dangling symlinks and unrelated applications.
            if let attributes = try? files.attributesOfItem(atPath: destination.path) {
                guard attributes[.type] as? FileAttributeType == .typeDirectory else {
                    throw InstallerFailure("安装位置不是普通应用目录，未进行替换。")
                }
                _ = try InstallerBundle.inspect(destination)
                try move(destination, backup)
                backedUp = true
            }
            try move(candidate, destination)
            installed = true
            try verify(destination)
            let result = try InstallerBundle.inspect(destination)
            guard result.version == expected.version, result.build == expected.build else {
                throw InstallerFailure("安装后的版本校验失败。")
            }
        } catch {
            do {
                if installed { try files.removeItem(at: destination) }
                if backedUp { try move(backup, destination) }
            } catch let restoreError {
                throw InstallerFailure("安装未完成，旧版恢复也未完成。旧版保留在 \(backup.path)。\n\(restoreError.localizedDescription)")
            }
            throw InstallerFailure("安装未完成，原有应用已保留。\n\(error.localizedDescription)")
        }
        if backedUp { try? files.removeItem(at: backup) }
    }
}

@MainActor
struct InstallerFlow {
    var validate: () throws -> Void
    var quitAndWait: () async throws -> Void
    var replace: () async throws -> Void
    var launch: () async throws -> Void

    func run() async throws {
        try validate()
        try await quitAndWait()
        try await replace()
        try await launch()
    }
}
