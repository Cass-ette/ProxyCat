import SwiftUI
import BatteryView

struct TrafficCapsuleView: View {
    let presentation: TrafficPresentation
    let isRefreshing: Bool
    let onRefresh: () -> Void
    @State private var showingDetails = false

    private var tint: Color {
        guard !presentation.isStale else { return .secondary }
        if presentation.isExpired { return .red }
        guard let fraction = presentation.fraction else { return .secondary }
        if fraction <= 0.1 { return .red }
        if fraction <= 0.2 { return .orange }
        return .green
    }

    private var expirationTint: Color {
        if presentation.expirationIsUrgent { return .red }
        if presentation.expirationNeedsAttention { return .orange }
        return .secondary
    }

    var body: some View {
        Button { showingDetails.toggle() } label: {
            HStack(spacing: 5) {
                Battery(.constant(Float(presentation.fraction ?? 0)),
                        .constant(presentation.fraction == nil ? .unknown : .unplugged))
                    .batteryStyle(SFSymbolStyle(TrafficBatteryColors(tint: tint), animation: nil))
                    .frame(width: 24, height: 12)
                    .accessibilityHidden(true)
                VStack(alignment: .trailing, spacing: 1) {
                    Text(isRefreshing && presentation.traffic == nil ? "查询中…" : caption)
                        .font(.system(size: 9))
                        .foregroundColor(.secondary)
                    if let amount = presentation.amount {
                        Text(amount)
                            .font(.system(size: 10, weight: .medium))
                            .monospacedDigit()
                            .foregroundColor(.primary)
                    }
                    if let expiration = presentation.expirationSummary {
                        Text(expiration)
                            .font(.system(size: 9))
                            .monospacedDigit()
                            .foregroundColor(expirationTint)
                    }
                }
                .lineLimit(1)
            }
            .padding(.vertical, 3)
            .contentShape(Rectangle())
        }
        .buttonStyle(PlainButtonStyle())
        .fixedSize()
        .help(presentation.help)
        .accessibilityLabel(["订阅流量，\(presentation.label)", presentation.expirationSummary].compactMap { $0 }.joined(separator: "，"))
        .popover(isPresented: $showingDetails, arrowEdge: .bottom) {
            details
        }
    }

    private var caption: String {
        if presentation.warning == .low || presentation.warning == .critical { return "剩余 · 偏低" }
        return presentation.caption
    }

    private var details: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("订阅流量").font(.headline)
            if let warning = presentation.warning {
                Label(warning.message, systemImage: "exclamationmark.circle.fill")
                    .foregroundColor(tint)
            }
            if let profile = presentation.profile {
                Text(profile.name).font(.caption).foregroundColor(.secondary).lineLimit(2)
                if let traffic = presentation.traffic {
                    detailRow("剩余", traffic.remaining.map(TrafficPresentation.bytes) ?? "未提供上限")
                    detailRow("已用", TrafficPresentation.bytes(traffic.used))
                    detailRow("总量", traffic.total > 0 ? TrafficPresentation.bytes(traffic.total) : "未提供上限")
                    detailRow("到期", traffic.expirationDate?.formatted(date: .abbreviated, time: .shortened) ?? "未提供")
                } else {
                    Text(profile.trafficCheckedAt == nil ? "刷新以查询套餐流量。" : "订阅未返回可用的流量信息。")
                        .foregroundColor(.secondary)
                }
                if let checkedAt = presentation.checkedAt {
                    Text("更新于 \(checkedAt.formatted(date: .abbreviated, time: .shortened))")
                        .font(.caption).foregroundColor(.secondary)
                }
                if let error = presentation.refreshError {
                    Text(error).font(.caption).foregroundColor(.orange)
                } else if presentation.isStale, presentation.traffic != nil {
                    Text("上次获取的数据，请刷新确认。")
                        .font(.caption).foregroundColor(.secondary)
                }
                Text("套餐用量由订阅服务提供，非本机实时统计。")
                    .font(.caption).foregroundColor(.secondary)
                Button(isRefreshing ? "刷新中…" : "刷新流量", action: onRefresh)
                    .disabled(isRefreshing)
            } else {
                Text("先导入或选择一个订阅。")
                    .foregroundColor(.secondary)
            }
        }
        .font(.system(size: 12))
        .padding(16)
        .frame(width: 235, alignment: .leading)
        .fixedSize(horizontal: false, vertical: true)
    }

    private func detailRow(_ title: String, _ value: String) -> some View {
        HStack {
            Text(title).foregroundColor(.secondary)
            Spacer()
            Text(value).monospacedDigit()
        }
    }
}

private struct TrafficBatteryColors: ColorConfiguration {
    let tint: Color
    func primary(for configuration: BatteryStyleConfiguration) -> Color { tint }
    func secondary(for configuration: BatteryStyleConfiguration) -> Color { tint }
    func tertiary(for configuration: BatteryStyleConfiguration) -> Color { .secondary.opacity(0.65) }
}
