// Regenerate background.png with: swift scripts/dmg/background.swift
import AppKit

let size = NSSize(width: 640, height: 480)
let bitmap = NSBitmapImageRep(
    bitmapDataPlanes: nil, pixelsWide: 1280, pixelsHigh: 960,
    bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true,
    isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0
)!
bitmap.size = size
NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: bitmap)
NSColor(calibratedRed: 0.955, green: 0.966, blue: 0.960, alpha: 1).setFill()
NSRect(origin: .zero, size: size).fill()

func centered(_ text: String, top: CGFloat, font: NSFont, color: NSColor) {
    let paragraph = NSMutableParagraphStyle()
    paragraph.alignment = .center
    (text as NSString).draw(in: NSRect(x: 24, y: size.height - top - 34, width: 592, height: 34),
                           withAttributes: [.font: font, .foregroundColor: color, .paragraphStyle: paragraph])
}

let titleFont = NSFont(name: "PingFangSC-Semibold", size: 28)!
let bodyFont = NSFont(name: "PingFangSC-Regular", size: 14)!
let smallFont = NSFont(name: "PingFangSC-Regular", size: 13)!
centered("ProxyCat", top: 32, font: titleFont, color: .black)
centered("拖到“应用程序”即可安装", top: 77,
         font: bodyFont, color: .darkGray)

func panel(top: CGFloat, height: CGFloat, fill: NSColor, border: NSColor) {
    let shape = NSBezierPath(roundedRect: NSRect(x: 36, y: size.height - top - height,
                                                width: 568, height: height),
                             xRadius: 22, yRadius: 22)
    fill.setFill()
    shape.fill()
    border.setStroke()
    shape.lineWidth = 1
    shape.stroke()
}

panel(top: 120, height: 144, fill: .white,
      border: NSColor(calibratedWhite: 0.87, alpha: 1))
panel(top: 300, height: 132,
      fill: NSColor(calibratedWhite: 1, alpha: 0.48),
      border: NSColor(calibratedWhite: 0.87, alpha: 0.75))

func label(_ text: String, x: CGFloat, top: CGFloat, font: NSFont, color: NSColor) {
    (text as NSString).draw(in: NSRect(x: x, y: size.height - top - 54,
                                     width: 365, height: 54),
                           withAttributes: [.font: font, .foregroundColor: color])
}
label("更新旧版？也可用安装程序", x: 208, top: 326,
      font: NSFont(name: "PingFangSC-Semibold", size: 14)!, color: .darkGray)
label("双击左侧图标，自动退出旧版并安装\n订阅和设置都会保留", x: 208, top: 354,
      font: smallFont, color: .gray)

// Finder uses top-left icon coordinates; AppKit uses bottom-left drawing.
let arrowY = size.height - 175
let arrow = NSBezierPath()
arrow.move(to: NSPoint(x: 296, y: arrowY))
arrow.line(to: NSPoint(x: 344, y: arrowY))
arrow.move(to: NSPoint(x: 333, y: arrowY + 10))
arrow.line(to: NSPoint(x: 344, y: arrowY))
arrow.line(to: NSPoint(x: 333, y: arrowY - 10))
arrow.lineWidth = 2
arrow.lineCapStyle = .round
arrow.lineJoinStyle = .round
NSColor(calibratedWhite: 0.60, alpha: 1).setStroke()
arrow.stroke()

NSGraphicsContext.restoreGraphicsState()
let output = URL(fileURLWithPath: #filePath).deletingLastPathComponent().appendingPathComponent("background.png")
try bitmap.representation(using: .png, properties: [:])!.write(to: output)
print("Created \(output.path)")
