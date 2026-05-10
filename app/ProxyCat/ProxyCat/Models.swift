import Foundation

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
