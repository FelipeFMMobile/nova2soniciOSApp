#!/usr/bin/env swift

import AppKit
import Foundation

let width = 1800
let height = 1650
let output = CommandLine.arguments.dropFirst().first ?? "docs/images/architecture-litellm.png"

let navy = NSColor(calibratedRed: 0.10, green: 0.16, blue: 0.25, alpha: 1)
let line = NSColor(calibratedRed: 0.29, green: 0.39, blue: 0.50, alpha: 1)
let border = NSColor(calibratedRed: 0.72, green: 0.78, blue: 0.85, alpha: 1)
let canvas = NSColor(calibratedRed: 0.965, green: 0.975, blue: 0.985, alpha: 1)
let localFill = NSColor(calibratedRed: 0.92, green: 0.95, blue: 0.975, alpha: 1)
let gatewayFill = NSColor(calibratedRed: 0.89, green: 0.96, blue: 0.95, alpha: 1)
let dockerFill = NSColor(calibratedRed: 0.94, green: 0.93, blue: 0.985, alpha: 1)
let awsFill = NSColor(calibratedRed: 1.0, green: 0.95, blue: 0.86, alpha: 1)
let white = NSColor.white
let dbFill = NSColor(calibratedRed: 0.89, green: 0.92, blue: 0.97, alpha: 1)
let purple = NSColor(calibratedRed: 0.36, green: 0.27, blue: 0.63, alpha: 1)

func rect(_ x: CGFloat, _ y: CGFloat, _ w: CGFloat, _ h: CGFloat) -> NSRect {
    NSRect(x: x, y: CGFloat(height) - y - h, width: w, height: h)
}

func point(_ x: CGFloat, _ y: CGFloat) -> NSPoint {
    NSPoint(x: x, y: CGFloat(height) - y)
}

func rounded(_ frame: NSRect, fill: NSColor, stroke: NSColor = border, radius: CGFloat = 18, width: CGFloat = 2) {
    let path = NSBezierPath(roundedRect: frame, xRadius: radius, yRadius: radius)
    fill.setFill()
    path.fill()
    stroke.setStroke()
    path.lineWidth = width
    path.stroke()
}

func text(_ value: String, x: CGFloat, y: CGFloat, w: CGFloat, h: CGFloat,
          size: CGFloat, weight: NSFont.Weight = .regular,
          color: NSColor = navy, align: NSTextAlignment = .center) {
    let style = NSMutableParagraphStyle()
    style.alignment = align
    style.lineBreakMode = .byWordWrapping
    style.lineSpacing = max(1, size * 0.12)
    let attributes: [NSAttributedString.Key: Any] = [
        .font: NSFont.systemFont(ofSize: size, weight: weight),
        .foregroundColor: color,
        .paragraphStyle: style
    ]
    (value as NSString).draw(in: rect(x, y, w, h), withAttributes: attributes)
}

func box(_ x: CGFloat, _ y: CGFloat, _ w: CGFloat, _ h: CGFloat,
         title: String, lines: [String], fill: NSColor = white,
         stroke: NSColor = border, titleSize: CGFloat = 24, bodySize: CGFloat = 18) {
    rounded(rect(x, y, w, h), fill: fill, stroke: stroke)
    text(title, x: x + 20, y: y + 20, w: w - 40, h: 34, size: titleSize, weight: .semibold)
    if !lines.isEmpty {
        text(lines.joined(separator: "\n"), x: x + 18, y: y + 62, w: w - 36, h: h - 70,
             size: bodySize)
    }
}

func arrowHead(from a: NSPoint, to b: NSPoint, color: NSColor = line) {
    let angle = atan2(b.y - a.y, b.x - a.x)
    let length: CGFloat = 12
    let spread: CGFloat = 0.55
    let head = NSBezierPath()
    head.move(to: b)
    head.line(to: NSPoint(x: b.x - length * cos(angle - spread), y: b.y - length * sin(angle - spread)))
    head.line(to: NSPoint(x: b.x - length * cos(angle + spread), y: b.y - length * sin(angle + spread)))
    head.close()
    color.setFill()
    head.fill()
}

func arrow(_ coords: [(CGFloat, CGFloat)], both: Bool = false, color: NSColor = line, width: CGFloat = 3) {
    guard coords.count >= 2 else { return }
    let points = coords.map { point($0.0, $0.1) }
    let path = NSBezierPath()
    path.move(to: points[0])
    for p in points.dropFirst() { path.line(to: p) }
    color.setStroke()
    path.lineWidth = width
    path.lineJoinStyle = .round
    path.stroke()
    arrowHead(from: points[points.count - 2], to: points.last!, color: color)
    if both { arrowHead(from: points[1], to: points[0], color: color) }
}

func pill(_ value: String, x: CGFloat, y: CGFloat, w: CGFloat, color: NSColor = navy) {
    rounded(rect(x, y, w, 31), fill: canvas, stroke: canvas, radius: 6, width: 0)
    text(value, x: x + 5, y: y + 4, w: w - 10, h: 24, size: 16, weight: .medium, color: color)
}

guard let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: width, pixelsHigh: height,
                                    bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true,
                                    isPlanar: false, colorSpaceName: .deviceRGB,
                                    bytesPerRow: 0, bitsPerPixel: 0),
      let context = NSGraphicsContext(bitmapImageRep: bitmap) else {
    fatalError("Could not create drawing context")
}

NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = context
canvas.setFill()
NSBezierPath(rect: NSRect(x: 0, y: 0, width: width, height: height)).fill()

// Header and external systems.
text("Nova Voice — arquitetura da POC com LiteLLM", x: 80, y: 44, w: 1000, h: 52,
     size: 38, weight: .bold, align: .left)
text("Gateway e orquestração em Go · LiteLLM Realtime · custos no Usage/Spend",
     x: 80, y: 102, w: 1250, h: 34, size: 22, align: .left)

box(235, 165, 620, 120, title: "App SwiftUI · macOS / iOS",
    lines: ["Microfone, reprodução e interface"], titleSize: 25, bodySize: 19)
box(1080, 145, 630, 145, title: "AWS · AMAZON BEDROCK",
    lines: ["Nova 2 Sonic"], fill: awsFill, stroke: NSColor(calibratedRed: 0.86, green: 0.57, blue: 0.18, alpha: 1),
    titleSize: 22, bodySize: 28)

// Local Mac boundary.
rounded(rect(45, 320, 1710, 1230), fill: localFill, stroke: border, radius: 28, width: 2)
text("AMBIENTE LOCAL NO MAC", x: 75, y: 345, w: 500, h: 36, size: 23, weight: .bold, align: .left)

// Go process.
rounded(rect(75, 395, 900, 455), fill: gatewayFill,
        stroke: NSColor(calibratedRed: 0.35, green: 0.64, blue: 0.62, alpha: 1), radius: 22, width: 2)
text("PROCESSO GATEWAY · GO · PORTA 8080", x: 100, y: 420, w: 650, h: 34,
     size: 22, weight: .bold, align: .left)
box(110, 475, 380, 130, title: "API HTTP / WebSocket",
    lines: ["Token local · WS /v1/voice", "/healthz · /v1/providers · /v1/metrics"], titleSize: 23, bodySize: 17)
box(555, 475, 380, 130, title: "Controle da conversa",
    lines: ["Sessões, turnos e timeouts", "Streaming e interrupção"], titleSize: 23, bodySize: 18)
box(110, 660, 380, 145, title: "Orquestrador de ferramentas",
    lines: ["Schemas e políticas do host", "Confirmações e idempotência"], titleSize: 22, bodySize: 18)
box(555, 660, 380, 145, title: "Adaptador Nova",
    lines: ["Áudio, transcrições e toolUse", "Mapeamento de eventos"], titleSize: 23, bodySize: 18)

// LiteLLM owns the cloud boundary and cost metadata.
box(1080, 475, 630, 165, title: "LiteLLM Realtime · porta 4000",
    lines: ["Chave virtual sts-go-gateway", "Perfil AWS somente leitura", "WebSocket → Bedrock / Nova"],
    titleSize: 23, bodySize: 18)
box(1080, 710, 630, 130, title: "PostgreSQL · Usage / Spend",
    lines: ["Modelo, chave, sessão e custo · sem conteúdo"], fill: dbFill, titleSize: 23, bodySize: 18)

// Docker / LiteLLM boundary.
rounded(rect(75, 900, 1635, 580), fill: dockerFill,
        stroke: NSColor(calibratedRed: 0.56, green: 0.48, blue: 0.76, alpha: 1), radius: 22, width: 2)
text("DOCKER LOCAL · LITELLM, POSTGRESQL E MCP", x: 100, y: 925, w: 720, h: 34,
     size: 22, weight: .bold, color: purple, align: .left)
box(110, 975, 500, 165, title: "LiteLLM MCP Gateway · porta 4000",
    lines: ["REST MCP · autenticação por service key", "Grants por servidor/tool · 60 RPM", "Encaminha envelope assinado nos arguments"],
    fill: white, stroke: purple, titleSize: 22, bodySize: 17)
box(665, 975, 390, 165, title: "Chave virtual",
    lines: ["Modelo nova-sonic", "/v1/realtime + MCPs autorizados"],
    fill: dbFill, stroke: purple, titleSize: 22, bodySize: 16)
box(1110, 975, 555, 165, title: "Painel administrativo local",
    lines: ["http://127.0.0.1:4000/ui", "Saúde · MCPs · chaves · permissões"],
    fill: white, stroke: purple, titleSize: 22, bodySize: 18)

box(110, 1210, 360, 115, title: "MCP Notes · Go",
    lines: ["create · list · delete"], titleSize: 22, bodySize: 18)
box(555, 1210, 400, 115, title: "MCP Agenda · Go",
    lines: ["create_event · cancel_event", "list_events · list_slots"], titleSize: 22, bodySize: 17)
box(110, 1370, 360, 85, title: "SQLite Notes",
    lines: ["Notas + ledger idempotente"], fill: dbFill, titleSize: 21, bodySize: 16)
box(555, 1370, 400, 85, title: "SQLite Agenda",
    lines: ["Eventos + ledger idempotente"], fill: dbFill, titleSize: 21, bodySize: 16)

// Data paths.
arrow([(545, 285), (545, 305), (60, 305), (60, 540), (110, 540)], both: true)
pill("PCM16 + eventos JSON", x: 315, y: 292, w: 215)
arrow([(490, 540), (555, 540)], both: true)
arrow([(745, 605), (745, 660)], both: true)
arrow([(620, 605), (620, 632), (300, 632), (300, 660)], both: true)
pill("toolUse / resultado", x: 360, y: 613, w: 190)
arrow([(935, 735), (1015, 735), (1015, 558), (1080, 558)], both: true)
pill("Realtime + service key", x: 895, y: 648, w: 190)
arrow([(1395, 475), (1395, 290)], both: true)
pill("Stream AWS autenticado", x: 1280, y: 360, w: 230)
arrow([(1395, 640), (1395, 710)])
pill("spend metadata", x: 1270, y: 660, w: 160)

// Governed MCP path.
arrow([(300, 805), (300, 875), (360, 875), (360, 975)], both: true, color: purple)
pill("MCP REST · service key · envelope HMAC", x: 300, y: 850, w: 355, color: purple)
arrow([(610, 1058), (665, 1058)], both: true, color: purple)
arrow([(1110, 1025), (1080, 1025), (1080, 950), (360, 950), (360, 975)], both: true, color: purple)
pill("API administrativa · localhost /ui", x: 720, y: 925, w: 295, color: purple)
arrow([(260, 1140), (260, 1210)], both: true, color: purple)
arrow([(470, 1110), (755, 1110), (755, 1210)], both: true, color: purple)
pill("JSON-RPC · stdio", x: 480, y: 1145, w: 175, color: purple)
arrow([(290, 1325), (290, 1370)], both: true)
arrow([(755, 1325), (755, 1370)], both: true)

// Footer.
text("A Nova escolhe as tools; o Go valida intenção, confirmação, schema e identidade da operação.",
     x: 80, y: 1572, w: 1640, h: 28, size: 20, weight: .semibold, align: .left)
text("Somente o LiteLLM possui credenciais AWS e registra Usage/Spend sem conteúdo da conversa.",
     x: 80, y: 1607, w: 1640, h: 28, size: 18, align: .left)

context.flushGraphics()
NSGraphicsContext.restoreGraphicsState()

let destination = URL(fileURLWithPath: output)
try FileManager.default.createDirectory(at: destination.deletingLastPathComponent(),
                                        withIntermediateDirectories: true)
guard let data = bitmap.representation(using: .png, properties: [:]) else {
    fatalError("Could not encode PNG")
}
try data.write(to: destination)
print(destination.path)
