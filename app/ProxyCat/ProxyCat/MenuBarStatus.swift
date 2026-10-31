import AppKit
import Combine

enum MenuBarProxyState: Equatable {
    case off, engineOnly, connected, direct, engineStopped

    init(running: Bool, enabled: Bool, mode: String) {
        if !enabled {
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
        case .connected, .direct: return "cat.fill"
        case .engineStopped: return "exclamationmark.triangle"
        }
    }

    var color: NSColor? {
        switch self {
        case .off, .engineOnly: return nil
        case .connected: return .systemGreen
        case .direct: return .systemBlue
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
        }
    }

    var title: String {
        switch self {
        case .off, .engineOnly: return "ProxyCat：代理已关闭"
        case .connected: return "ProxyCat：代理已开启"
        case .direct: return "ProxyCat：直连模式"
        case .engineStopped: return "ProxyCat：代理引擎已停止"
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

@MainActor
final class MenuBarStatusObserver {
    private var observation: AnyCancellable?

    init(viewModel: StatusViewModel, onChange: @escaping (MenuBarProxyState) -> Void) {
        observation = Publishers.CombineLatest3(viewModel.$coreRunning, viewModel.$systemProxyEnabled, viewModel.$currentMode)
            .map { MenuBarProxyState(running: $0, enabled: $1, mode: $2) }
            .removeDuplicates()
            .receive(on: DispatchQueue.main)
            .sink(receiveValue: onChange)
    }
}
