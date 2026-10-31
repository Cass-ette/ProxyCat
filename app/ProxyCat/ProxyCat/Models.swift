import Foundation

enum CampusNetworkAction: String, CaseIterable, Identifiable {
    case prompt
    case disconnect

    var id: String { rawValue }

    var title: String {
        switch self {
        case .prompt:
            return "提示"
        case .disconnect:
            return "自动断开"
        }
    }
}

struct CoreStatus: Codable {
    let running: Bool
    let pid: Int
}

struct SystemProxyStatus: Codable {
    let httpEnabled: Bool
    let httpHost: String
    let httpPort: Int
    let httpsEnabled: Bool
    let httpsHost: String
    let httpsPort: Int
    let socksEnabled: Bool
    let socksHost: String
    let socksPort: Int
}

struct TestResult: Codable {
    let googleOK: Bool
    let githubOK: Bool
    let error: String?
}

struct ProxyDelayResult: Codable {
    let delay: Int
    let error: String?
}

struct ModeStatus: Codable {
    let mode: String
}

struct ProxyGroup: Codable, Identifiable {
    var id: String { name }

    let name: String
    let type: String
    let now: String
    let all: [String]
}

struct DiagnoseReport: Codable {
    let app: String
    let milestone: String
    let checks: [CheckResult]
}

struct CheckResult: Codable {
    let name: String
    let status: String
    let message: String
    let suggestedFix: String?
}

struct UpdateEvent: Codable {
    let stage: String
    let message: String
    let progress: Int?
    let newVersion: String?
    var releaseVersion: String? = nil
    var releaseNotes: String? = nil
    var releaseURL: String? = nil
    var publishedAt: String? = nil
    var recheck: Bool? = nil
    var details: String? = nil
}

struct ReleaseNotesSection: Identifiable {
    let id: Int
    let title: String
    let items: [String]

    static func parse(_ markdown: String) -> [ReleaseNotesSection] {
        var sections: [ReleaseNotesSection] = []
        var title = "更新内容"
        var items: [String] = []
        func flush() {
            if !items.isEmpty {
                sections.append(ReleaseNotesSection(id: sections.count, title: title, items: items))
                items = []
            }
        }
        for raw in markdown.components(separatedBy: .newlines) {
            let line = raw.trimmingCharacters(in: .whitespaces)
            if line.hasPrefix("#") {
                flush()
                title = String(line.drop(while: { $0 == "#" || $0 == " " }))
            } else if line.hasPrefix("- ") || line.hasPrefix("* ") {
                items.append(String(line.dropFirst(2)))
            } else if !line.isEmpty {
                if raw.hasPrefix("  "), let last = items.popLast() {
                    items.append(last + " " + line)
                } else {
                    items.append(line)
                }
            }
        }
        flush()
        return sections
    }
}

struct Profile: Codable, Identifiable {
    let id: String
    let name: String
    let url: String
    let createdAt: String
    let updatedAt: String
    let active: Bool
}
