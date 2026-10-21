// Regenerate background.png with: swift scripts/dmg/background.swift
import AppKit

let size = NSSize(width: 660, height: 440)
let bitmap = NSBitmapImageRep(
    bitmapDataPlanes: nil, pixelsWide: 1320, pixelsHigh: 880,
    bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true,
    isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0
)!
bitmap.size = size
NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: bitmap)
NSGradient(starting: NSColor(calibratedRed: 0.98, green: 0.98, blue: 1, alpha: 1),
           ending: NSColor(calibratedRed: 0.92, green: 0.94, blue: 0.99, alpha: 1))!
    .draw(in: NSRect(origin: .zero, size: size), angle: 90)

func centered(_ text: String, top: CGFloat, font: NSFont, color: NSColor) {
    let paragraph = NSMutableParagraphStyle()
    paragraph.alignment = .center
    (text as NSString).draw(in: NSRect(x: 20, y: size.height - top - 34, width: 620, height: 34),
                           withAttributes: [.font: font, .foregroundColor: color, .paragraphStyle: paragraph])
}

centered("安装 ProxyCat", top: 35, font: .systemFont(ofSize: 28, weight: .semibold),
         color: NSColor(calibratedRed: 0.15, green: 0.18, blue: 0.28, alpha: 1))
centered("将左侧的小猫拖到右侧“应用程序”", top: 81,
         font: .systemFont(ofSize: 15), color: .darkGray)

// Finder uses top-left icon coordinates; AppKit uses bottom-left drawing.
let arrowY = size.height - 190
let arrow = NSBezierPath()
arrow.move(to: NSPoint(x: 295, y: arrowY))
arrow.line(to: NSPoint(x: 365, y: arrowY))
arrow.move(to: NSPoint(x: 350, y: arrowY + 14))
arrow.line(to: NSPoint(x: 365, y: arrowY))
arrow.line(to: NSPoint(x: 350, y: arrowY - 14))
arrow.lineWidth = 4
arrow.lineCapStyle = .round
arrow.lineJoinStyle = .round
NSColor(calibratedRed: 0.49, green: 0.52, blue: 0.66, alpha: 1).setStroke()
arrow.stroke()

centered("安装前请先退出旧版 ProxyCat", top: 312,
         font: .systemFont(ofSize: 14, weight: .medium), color: .darkGray)
centered("完成后从“应用程序”打开，小猫会出现在顶部菜单栏", top: 349,
         font: .systemFont(ofSize: 13), color: .gray)

NSGraphicsContext.restoreGraphicsState()
let output = URL(fileURLWithPath: #filePath).deletingLastPathComponent().appendingPathComponent("background.png")
try bitmap.representation(using: .png, properties: [:])!.write(to: output)
print("Created \(output.path)")
