import AppKit
import Combine

enum MenuBarProxyState: Equatable {
    case off, engineOnly, connected, direct, engineStopped, tun, tunDirect

    init(running: Bool, enabled: Bool, mode: String, tunEnabled: Bool = false) {
        if running && tunEnabled {
            self = mode == "direct" ? .tunDirect : .tun
        } else if !enabled {
            self = running ? .engineOnly : .off
        } else if !running {
            self = .engineStopped
        } else {
            self = mode == "direct" ? .direct : .connected
        }
    }

    var symbol: String {
        switch self {
        case .off, .engineOnly: return "cat"
        case .connected, .direct, .tun, .tunDirect: return "cat.fill"
        case .engineStopped: return "exclamationmark.triangle"
        }
    }

    var color: NSColor? {
        switch self {
        case .off, .engineOnly: return nil
        case .connected, .tun: return .systemGreen
        case .direct, .tunDirect: return .systemBlue
        case .engineStopped: return .systemOrange
        }
    }

    var description: String {
        switch self {
        case .off: return "ProxyCat：系统代理已关闭"
        case .engineOnly: return "ProxyCat：系统代理已关闭，代理引擎运行中"
        case .connected: return "ProxyCat：系统代理已开启"
        case .direct: return "ProxyCat：直连模式，系统代理已开启"
        case .engineStopped: return "ProxyCat：系统代理已开启，但代理引擎已停止"
        case .tun: return "ProxyCat：TUN 已开启"
        case .tunDirect: return "ProxyCat：TUN 已开启，直连模式"
        }
    }

    var title: String {
        switch self {
        case .off, .engineOnly: return "ProxyCat：代理已关闭"
        case .connected: return "ProxyCat：代理已开启"
        case .direct: return "ProxyCat：直连模式"
        case .engineStopped: return "ProxyCat：代理引擎已停止"
        case .tun: return "ProxyCat：TUN 已开启"
        case .tunDirect: return "ProxyCat：TUN · 直连模式"
        }
    }

    func image() -> NSImage? {
        let symbolImage = NSImage(systemSymbolName: symbol, accessibilityDescription: description)
        let image = color.flatMap {
            symbolImage?.withSymbolConfiguration(NSImage.SymbolConfiguration(paletteColors: [$0]))
        } ?? symbolImage
        image?.size = NSSize(width: 18, height: 18)
        // Template images follow macOS contrast in the off state; active
        // images keep their semantic color in both light and dark menus.
        image?.isTemplate = color == nil
        return image
    }
}

struct MenuBarIndicator: Equatable {
    let state: MenuBarProxyState
    var warning: TrafficWarning? = nil
    var trafficSummary: String? = nil
    var transitionDescription: String? = nil

    var description: String {
        ([transitionDescription ?? state.description] + [warning?.message, trafficSummary].compactMap { $0 }).joined(separator: "\n")
    }

    func image() -> NSImage? {
        guard let warning else { return state.image() }
        let symbol = NSImage(systemSymbolName: state.symbol, accessibilityDescription: description)?
            .withSymbolConfiguration(NSImage.SymbolConfiguration(paletteColors: [state.color ?? .labelColor]))
        let badge: NSColor = warning == .low ? .systemYellow : warning == .critical ? .systemOrange : .systemRed
        let image = NSImage(size: NSSize(width: 23, height: 18), flipped: false) { _ in
            symbol?.draw(in: NSRect(x: 0, y: 0, width: 18, height: 18))
            let dot = NSBezierPath(ovalIn: NSRect(x: 16, y: 1, width: 6, height: 6))
            NSColor.windowBackgroundColor.setStroke()
            dot.lineWidth = 1.5
            dot.stroke()
            badge.setFill()
            dot.fill()
            return true
        }
        image.isTemplate = false
        return image
    }
}

@MainActor
final class MenuBarStatusObserver {
    private var observation: AnyCancellable?

    init(viewModel: StatusViewModel, onChange: @escaping (MenuBarIndicator) -> Void) {
        let state = Publishers.CombineLatest4(viewModel.$coreRunning, viewModel.$systemProxyEnabled, viewModel.$currentMode, viewModel.$tunEnabled)
            .map { MenuBarProxyState(running: $0, enabled: $1, mode: $2, tunEnabled: $3) }
        let traffic = Publishers.CombineLatest3(viewModel.$profiles, viewModel.$activeProfileID, viewModel.$trafficRefreshError)
            .map { profiles, activeID, error in
                TrafficPresentation(profile: profiles.first { $0.id == activeID }, refreshError: error)
            }
        let transition = Publishers.CombineLatest(viewModel.$tunChangeTarget, viewModel.$tunProgress)
            .map { target, progress -> String? in
                guard let target else { return nil }
                return "\(target ? "正在开启 TUN" : "正在关闭 TUN") · \(progress?.message ?? "正在准备")"
            }
        observation = Publishers.CombineLatest3(state, traffic, transition)
            .map { state, traffic, transition in
                MenuBarIndicator(state: state, warning: traffic.warning,
                                 trafficSummary: traffic.traffic == nil ? nil : traffic.label,
                                 transitionDescription: transition)
            }
            .removeDuplicates()
            .receive(on: DispatchQueue.main)
            .sink(receiveValue: onChange)
    }
}
