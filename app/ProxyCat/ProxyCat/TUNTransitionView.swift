import SwiftUI

struct TUNTransitionView: View {
    let progress: TUNProgress?
    let startedAt: Date?
    let enabling: Bool

    private var steps: [String] { enabling ? ["准备", "交接", "验证"] : ["隧道退出", "端口释放", "普通代理"] }
    private var step: Int {
        switch progress?.stage {
        case "verify", "restore": return 2
        case "release", "start": return 1
        case "stop": return enabling ? 1 : 0
        default: return 0
        }
    }

    var body: some View {
        TimelineView(.animation(minimumInterval: 1.0 / 30)) { context in
            VStack(alignment: .leading, spacing: 7) {
                HStack(spacing: 7) {
                    Circle().trim(from: 0, to: 0.7)
                        .stroke(Color.accentColor, style: StrokeStyle(lineWidth: 1.8, lineCap: .round))
                        .frame(width: 12, height: 12)
                        .rotationEffect(.degrees(context.date.timeIntervalSince1970.truncatingRemainder(dividingBy: 1) * 360))
                        .accessibilityHidden(true)
                    Text(enabling ? "正在开启 TUN" : "正在关闭 TUN")
                        .font(.system(size: 11, weight: .medium))
                    Spacer(minLength: 0)
                    Text("已等待 \(max(0, Int(context.date.timeIntervalSince(startedAt ?? context.date)))) 秒")
                        .font(.system(size: 10)).monospacedDigit().foregroundColor(.secondary)
                }
                HStack(spacing: 5) {
                    ForEach(Array(steps.enumerated()), id: \.offset) { index, title in
                        if index > 0 {
                            Image(systemName: "chevron.right").font(.system(size: 7)).foregroundColor(.secondary)
                        }
                        HStack(spacing: 3) {
                            Image(systemName: index < step ? "checkmark.circle.fill" : "circle.fill")
                                .font(.system(size: index < step ? 10 : 5))
                            Text(title).font(.system(size: 10))
                        }
                        .foregroundColor(index <= step ? .accentColor : .secondary)
                    }
                }
                Text(progress?.message ?? "正在准备切换")
                    .font(.system(size: 11))
                Text(progress?.detail ?? "可收起菜单，完成后会更新状态。")
                    .font(.system(size: 10)).foregroundColor(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .padding(10)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color.accentColor.opacity(0.07))
            .cornerRadius(7)
        }
        .accessibilityElement(children: .combine)
    }
}
