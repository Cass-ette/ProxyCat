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
    var traffic: SubscriptionTraffic? = nil
    var trafficCheckedAt: String? = nil
}

struct SubscriptionTraffic: Codable {
    let upload: Int64
    let download: Int64
    let total: Int64
    var expire: Int64? = nil

    var isValid: Bool { upload >= 0 && download >= 0 && total >= 0 && (expire ?? 0) >= 0 }
    var used: Int64 {
        let sum = upload.addingReportingOverflow(download)
        return sum.overflow ? Int64.max : sum.partialValue
    }
    var remaining: Int64? { total > 0 ? max(0, total - used) : nil }
    var remainingFraction: Double? {
        guard let remaining else { return nil }
        return min(1, max(0, Double(remaining) / Double(total)))
    }
    var expirationDate: Date? {
        guard let expire, expire > 0 else { return nil }
        return Date(timeIntervalSince1970: Double(expire))
    }
}

struct SubscriptionTrafficSnapshot: Codable {
    let profileID: String
    let traffic: SubscriptionTraffic?
    let checkedAt: String
}

enum TrafficWarning: Equatable {
    case low, critical, exhausted, expired

    var message: String {
        switch self {
        case .low: return "订阅剩余流量 ≤20%"
        case .critical: return "订阅剩余流量 ≤10%"
        case .exhausted: return "订阅流量已用完"
        case .expired: return "订阅已到期"
        }
    }
}

struct TrafficPresentation {
    let profile: Profile?
    var refreshError: String? = nil
    var now = Date()

    var traffic: SubscriptionTraffic? {
        guard let traffic = profile?.traffic, traffic.isValid else { return nil }
        return traffic
    }
    var checkedAt: Date? {
        guard let value = profile?.trafficCheckedAt else { return nil }
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = formatter.date(from: value) { return date }
        formatter.formatOptions = [.withInternetDateTime]
        return formatter.date(from: value)
    }
    var isStale: Bool {
        refreshError != nil || checkedAt.map { now.timeIntervalSince($0) >= 30 * 60 } != false
    }
    var isExpired: Bool { traffic?.expirationDate.map { $0 <= now } ?? false }
    var expirationSummary: String? {
        guard let traffic else { return nil }
        guard let expiration = traffic.expirationDate else { return "到期时间未提供" }
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "zh_CN")
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.dateFormat = Calendar.current.isDate(expiration, equalTo: now, toGranularity: .year) ? "M/d" : "yyyy/M/d"
        let date = formatter.string(from: expiration)
        if isStale { return "上次：\(date) 到期" }
        let remaining = expiration.timeIntervalSince(now)
        if remaining <= 0 { return "\(date) 已到期" }
        if remaining < 24 * 60 * 60 { return "\(date) 到期 · 不足1天" }
        let days = Int(ceil(remaining / (24 * 60 * 60)))
        return "\(date) 到期 · 剩\(days)天"
    }
    var expirationNeedsAttention: Bool {
        guard !isStale, let expiration = traffic?.expirationDate else { return false }
        return expiration.timeIntervalSince(now) <= 7 * 24 * 60 * 60
    }
    var expirationIsUrgent: Bool {
        guard !isStale, let expiration = traffic?.expirationDate else { return false }
        return expiration.timeIntervalSince(now) <= 24 * 60 * 60
    }
    var fraction: Double? { isExpired ? 0 : traffic?.remainingFraction }
    var warning: TrafficWarning? {
        guard !isStale, traffic != nil else { return nil }
        if isExpired { return .expired }
        guard let fraction else { return nil }
        if fraction == 0 { return .exhausted }
        if fraction <= 0.1 { return .critical }
        if fraction <= 0.2 { return .low }
        return nil
    }
    var caption: String {
        guard profile != nil else { return "未选订阅" }
        guard let traffic else { return "流量未知" }
        if isExpired { return isStale ? "上次已到期" : "套餐已到期" }
        if traffic.remaining != nil { return isStale ? "上次剩余" : "剩余" }
        return isStale ? "上次已用" : "已用"
    }
    var amount: String? {
        guard let traffic else { return nil }
        guard let remaining = traffic.remaining else { return Self.bytes(traffic.used) }
        let remainingText = Self.bytes(remaining)
        let totalText = Self.bytes(traffic.total)
        let unit = totalText.split(separator: " ").last.map(String.init) ?? ""
        if remainingText.hasSuffix(" " + unit) {
            return "\(remainingText.dropLast(unit.count + 1)) / \(totalText)"
        }
        return "\(remainingText) / \(totalText)"
    }
    var label: String {
        amount.map { "\(caption) \($0)" } ?? caption
    }
    var help: String {
        guard let profile else { return "选择订阅后查看套餐流量。" }
        var lines = [profile.name]
        if let warning { lines.append(warning.message) }
        if let traffic {
            if let remaining = traffic.remaining { lines.append("剩余 \(Self.bytes(remaining)) / 总量 \(Self.bytes(traffic.total))") }
            else { lines.append("订阅未提供流量上限") }
            lines.append("已用 \(Self.bytes(traffic.used))")
            if let expiration = traffic.expirationDate {
                lines.append("到期 \(expiration.formatted(date: .abbreviated, time: .shortened))")
            }
        } else {
            lines.append(profile.trafficCheckedAt == nil ? "点击查询订阅流量" : "订阅未返回可用的流量信息")
        }
        if isStale, traffic != nil { lines.append("上次获取的数据，请刷新确认") }
        if let refreshError { lines.append(refreshError) }
        lines.append("点击查看详情与刷新")
        return lines.joined(separator: "\n")
    }
    static func bytes(_ count: Int64) -> String {
        let units = ["B", "KB", "MB", "GB", "TB", "PB", "EB"]
        var value = Double(max(0, count))
        var index = 0
        while value >= 1024 && index < units.count - 1 { value /= 1024; index += 1 }
        let amount = value >= 100 || index == 0 ? String(format: "%.0f", value) : String(format: "%.1f", value)
        return "\(amount.hasSuffix(".0") ? String(amount.dropLast(2)) : amount) \(units[index])"
    }
}

struct TUNProgress: Codable, Equatable {
    let stage: String
    let message: String

    var detail: String {
        switch stage {
        case "prepare": return "准备完成后才会交接现有连接。"
        case "authorize": return "仅首次安装或助手更新需要授权，日常开关无需重复输入密码。"
        case "release": return "隧道已退出；系统仍在释放端口，最长等待 45 秒。"
        case "restore": return "正在恢复原模式与节点，并确认代理端口可用。"
        case "cleanup": return "启动未完成，正在恢复原连接。"
        default: return "切换仍在进行，可收起菜单，完成后会更新状态。"
        }
    }
}

enum ProxyDisplay {
    static func name(_ raw: String) -> String {
        let name = raw.replacingOccurrences(of: "\u{1F1F9}\u{1F1FC}", with: "\u{1F1E8}\u{1F1F3}")
        let taiwan = name.contains("台湾") || name.contains("台灣") || name.localizedCaseInsensitiveContains("Taiwan")
        return taiwan && !name.contains("\u{1F1E8}\u{1F1F3}") ? "\u{1F1E8}\u{1F1F3} " + name : name
    }
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
        case "proxy": return "代理 · \(ProxyDisplay.name(node))"
        case "direct": return "直连"
        case "reject": return "规则拒绝"
        default: return "路径未确认"
        }
    }

    var reason: String {
        if let description = RoutingRule.describe(rule, payload: rulePayload) { return description }
        // These are mode facts, not a substitute for missing socket evidence.
        if route != "unknown" && mode == "global" { return "按全局模式" }
        if route != "unknown" && mode == "direct" { return "按直连模式" }
        return "未捕获到这次请求的路由证据"
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

enum RoutingRule {
    static func describe(_ rule: String, payload: String) -> String? {
        switch rule.lowercased() {
        case "match", "final": return "按配置的默认规则"
        case "global": return "按全局模式"
        case "direct": return "按直连模式"
        case "geoip" where payload.uppercased() == "CN": return "命中国内 IP 规则"
        case "geoip": return "命中 IP 归属规则（\(payload)）"
        case "domain", "domainsuffix", "domainkeyword", "domainregex": return "命中域名规则（\(payload)）"
        case "ruleset": return "命中规则集（\(payload)）"
        case "processname", "processpath", "processnameregex", "processpathregex": return "命中应用规则（\(payload)）"
        case "": return nil
        default: return "命中 \(rule) 规则\(payload.isEmpty ? "" : "（\(payload)）")"
        }
    }
}

struct ConnectionSnapshot: Codable {
    let observedAt: String
    let connections: [ConnectionRecord]
}

struct ConnectionRecord: Codable, Identifiable {
    let id: String
    let app: String
    let host: String
    let destinationPort: String
    let network: String
    let inbound: String
    let route: String
    let rule: String
    let rulePayload: String
    let chains: [String]
    let node: String
    let startedAt: String

    var appLabel: String { app.isEmpty ? "未识别应用" : app }
    var destination: String { host.isEmpty ? "未知目标" : host }
    var routeLabel: String {
        switch route {
        case "direct": return "直连"
        case "proxy": return node.isEmpty ? "代理" : "\(ProxyDisplay.name(node))（代理）"
        case "reject": return "规则拒绝"
        default: return "路径未确认"
        }
    }
    var reason: String { RoutingRule.describe(rule, payload: rulePayload) ?? "引擎未提供命中规则" }
    var searchText: String { [appLabel, destination, node, routeLabel, reason].joined(separator: " ") }
}

struct ConnectionLogEntry: Identifiable {
    var id: String { connection.id }
    var connection: ConnectionRecord
    var isActive: Bool
    let firstSeen: Date
    var lastSeen: Date
}

struct ConnectionAppGroup: Identifiable {
    var id: String { name }
    let name: String
    let entries: [ConnectionLogEntry]
    var activeCount: Int { entries.filter(\.isActive).count }
}

struct ConnectionLog {
    static let capacity = 300
    private(set) var entries: [ConnectionLogEntry] = []
    private(set) var updatedAt: Date?

    mutating func observe(_ snapshot: ConnectionSnapshot, at date: Date) {
        var previous = Dictionary(entries.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        var active: [ConnectionLogEntry] = []
        var seen = Set<String>()
        for connection in snapshot.connections where seen.insert(connection.id).inserted {
            let old = previous.removeValue(forKey: connection.id)
            active.append(ConnectionLogEntry(connection: connection, isActive: true,
                                             firstSeen: old?.firstSeen ?? date, lastSeen: date))
        }
        let ended = previous.values.map { old -> ConnectionLogEntry in
            var entry = old
            entry.isActive = false
            return entry
        }
        entries = Array((active + ended).sorted {
            if $0.isActive != $1.isActive { return $0.isActive }
            if $0.lastSeen != $1.lastSeen { return $0.lastSeen > $1.lastSeen }
            if $0.firstSeen != $1.firstSeen { return $0.firstSeen > $1.firstSeen }
            return $0.id < $1.id
        }.prefix(Self.capacity))
        updatedAt = date
    }

    mutating func clearEnded() { entries.removeAll { !$0.isActive } }

    func groups(matching query: String) -> [ConnectionAppGroup] {
        let query = query.trimmingCharacters(in: .whitespacesAndNewlines)
        let matches = entries.filter { query.isEmpty || $0.connection.searchText.localizedCaseInsensitiveContains(query) }
        return Dictionary(grouping: matches, by: { $0.connection.appLabel })
            .map { ConnectionAppGroup(name: $0.key, entries: $0.value) }
            .sorted {
                if ($0.name == "未识别应用") != ($1.name == "未识别应用") { return $1.name == "未识别应用" }
                return $0.name.localizedStandardCompare($1.name) == .orderedAscending
            }
    }
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
