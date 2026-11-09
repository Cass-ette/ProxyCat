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
    let tunEnabled: Bool?
    let tunDevice: String?
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

struct WebsiteCheckResult: Codable {
    let host: String
    let scheme: String
    let outcome: String
    let statusCode: Int
    let elapsedMS: Int
    let route: String
    let rule: String
    let rulePayload: String
    let chains: [String]
    let node: String
    let group: String
    let mode: String
    let observedAt: String

    var routeLabel: String {
        switch route {
        case "proxy": return "代理 · \(node)"
        case "direct": return "直连"
        case "reject": return "规则拒绝"
        default: return "路径未确认"
        }
    }

    var reason: String {
        switch rule.lowercased() {
        case "match", "final": return "按配置的默认规则"
        case "global": return "按全局模式"
        case "direct": return "按直连模式"
        case "geoip" where rulePayload.uppercased() == "CN": return "命中国内 IP 规则"
        case "geoip": return "命中 IP 归属规则（\(rulePayload)）"
        case "domain", "domainsuffix", "domainkeyword", "domainregex": return "命中域名规则（\(rulePayload)）"
        case "ruleset": return "命中规则集（\(rulePayload)）"
        case "":
            // These are mode facts, not a substitute for missing socket evidence.
            if route != "unknown" && mode == "global" { return "按全局模式" }
            if route != "unknown" && mode == "direct" { return "按直连模式" }
            return "未捕获到这次请求的路由证据"
        default: return "命中 \(rule) 规则\(rulePayload.isEmpty ? "" : "（\(rulePayload)）")"
        }
    }

    var outcomeLabel: String {
        switch outcome {
        case "responded": return "收到网页响应（HTTP \(statusCode)）"
        case "redirect": return "收到跳转响应（HTTP \(statusCode)，未跟随跳转）"
        case "denied": return "访问受限（HTTP \(statusCode)），不能据此判断节点断线"
        case "httpError": return "收到 HTTP \(statusCode) 错误响应"
        case "engineUnavailable": return "无法连接代理引擎"
        case "proxyUnavailable": return "本地代理端口不可用"
        case "certificate": return "证书验证失败"
        case "tlsFailed": return "HTTPS 握手失败"
        case "tunnelFailed": return "代理未能建立 HTTPS 连接"
        case "timeout": return "等待响应超时，原因尚未确定"
        default: return "连接中断，未收到网页响应"
        }
    }

    var summary: String { "这次检测\(route == "unknown" ? "路径未确认" : "走\(routeLabel)")，\(reason)；\(outcomeLabel)。" }
    var connectionFailed: Bool { ["timeout", "connectionFailed", "tlsFailed", "tunnelFailed"].contains(outcome) }
}

struct NodeChangeNotice {
    let message: String
    let observedAt: Date
}

extension ProxyDelayResult {
    var quality: String {
        if error != nil { return "不可用" }
        if delay <= 0 { return "未测试" }
        if delay <= 100 { return "快" }
        if delay <= 250 { return "一般" }
        return "慢"
    }
    var detail: String { error != nil ? "本次测试失败；不代表所有网站均不可达" : delay > 0 ? "\(delay) ms；仅为测试目标的响应延迟，不代表带宽" : "尚无有效测试结果" }
}
