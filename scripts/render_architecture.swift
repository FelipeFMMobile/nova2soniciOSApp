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
let futureFill = NSColor(calibratedRed: 0.95, green: 0.96, blue: 0.97, alpha: 1)
let futureLine = NSColor(calibratedRed: 0.48, green: 0.52, blue: 0.58, alpha: 1)

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

func arrow(_ coords: [(CGFloat, CGFloat)], both: Bool = false, color: NSColor = line,
           width: CGFloat = 3, dashed: Bool = false) {
    guard coords.count >= 2 else { return }
    let points = coords.map { point($0.0, $0.1) }
    let path = NSBezierPath()
    path.move(to: points[0])
    for p in points.dropFirst() { path.line(to: p) }
    color.setStroke()
    path.lineWidth = width
    path.lineJoinStyle = .round
    if dashed { path.setLineDash([10, 7], count: 2, phase: 0) }
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
text("STS Model — arquitetura modular orientada a domínios", x: 80, y: 44, w: 1250, h: 52,
     size: 38, weight: .bold, align: .left)
text("Aplicações independentes · contratos junto dos provedores · integrações no agente",
     x: 80, y: 102, w: 1500, h: 34, size: 22, align: .left)

box(185, 160, 700, 120, title: "Clientes de voz · SwiftUI e CLI Go",
    lines: ["Microfone, reprodução e protocolo /v1/voice"], titleSize: 25, bodySize: 19)
box(1110, 145, 600, 140, title: "AWS · AMAZON BEDROCK",
    lines: ["Nova 2 Sonic"], fill: awsFill,
    stroke: NSColor(calibratedRed: 0.86, green: 0.57, blue: 0.18, alpha: 1),
    titleSize: 22, bodySize: 28)

// Local monorepo boundary.
rounded(rect(45, 315, 1710, 1235), fill: localFill, stroke: border, radius: 28, width: 2)
text("AMBIENTE LOCAL · MONOREPO POLIGLOTA", x: 75, y: 338, w: 600, h: 36,
     size: 23, weight: .bold, align: .left)

// Voice gateway: public transport edge.
rounded(rect(75, 390, 480, 490), fill: gatewayFill,
        stroke: NSColor(calibratedRed: 0.35, green: 0.64, blue: 0.62, alpha: 1), radius: 22, width: 2)
text("APP VOICE-GATEWAY · GO · :8080", x: 100, y: 413, w: 420, h: 34,
     size: 19, weight: .bold, align: .left)
box(105, 470, 420, 125, title: "API de voz",
    lines: ["HTTP / WebSocket · token local", "contracts/voice/v1"], titleSize: 23, bodySize: 17)
box(105, 645, 420, 155, title: "Borda AgentSession",
    lines: ["Autenticação · limites · heartbeat", "Voice v1 ↔ agentwire"], titleSize: 22, bodySize: 17)
rounded(rect(105, 820, 420, 40), fill: white,
        stroke: NSColor(calibratedRed: 0.35, green: 0.64, blue: 0.62, alpha: 1), radius: 9, width: 1.5)
text("in-process padrão · remoto opt-in", x: 120, y: 828, w: 390, h: 24, size: 16, weight: .medium)

// Personal assistant owns semantic conversation and integrations.
rounded(rect(575, 390, 420, 490), fill: gatewayFill,
        stroke: NSColor(calibratedRed: 0.35, green: 0.64, blue: 0.62, alpha: 1), radius: 22, width: 2)
text("APP PERSONAL-ASSISTANT-AGENT", x: 600, y: 413, w: 370, h: 34,
     size: 18, weight: .bold, align: .left)
box(605, 470, 360, 125, title: "Conversa + Realtime",
    lines: ["Prompt · histórico · renovação", "VAD · turnos · interrupção"], titleSize: 20, bodySize: 16)
box(605, 645, 360, 155, title: "Actions + MCP",
    lines: ["Autorização · confirmação", "Idempotência · Host Context"], titleSize: 21, bodySize: 17)
rounded(rect(605, 820, 360, 40), fill: white,
        stroke: NSColor(calibratedRed: 0.35, green: 0.64, blue: 0.62, alpha: 1), radius: 9, width: 1.5)
text("prompts + config/integrations", x: 620, y: 828, w: 330, h: 24, size: 16, weight: .medium)

// LiteLLM owns cloud credentials and platform metadata; it is not a business domain.
rounded(rect(1050, 390, 660, 490), fill: dockerFill,
        stroke: NSColor(calibratedRed: 0.56, green: 0.48, blue: 0.76, alpha: 1), radius: 22, width: 2)
text("PLATAFORMA LOCAL · LITELLM · :4000", x: 1075, y: 413, w: 570, h: 34,
     size: 22, weight: .bold, color: purple, align: .left)
box(1080, 470, 600, 170, title: "Realtime + MCP Gateway",
    lines: ["Chave virtual e grants por servidor/tool", "WebSocket → Bedrock · MCP → aplicações", "Perfil AWS somente leitura"],
    fill: white, stroke: purple, titleSize: 23, bodySize: 18)
box(1080, 700, 285, 120, title: "PostgreSQL",
    lines: ["Chaves + Usage/Spend", "sem conteúdo"], fill: dbFill, titleSize: 22, bodySize: 16)
box(1395, 700, 285, 120, title: "Painel local",
    lines: ["/ui · saúde · MCPs", "chaves · permissões"], fill: white, titleSize: 22, bodySize: 16)

// Independent MCP applications. Each application owns its contract, rules and data.
rounded(rect(75, 920, 1635, 550), fill: dockerFill,
        stroke: NSColor(calibratedRed: 0.56, green: 0.48, blue: 0.76, alpha: 1), radius: 22, width: 2)
text("APLICAÇÕES MCP INDEPENDENTES · CONTRATOS E DADOS PRÓPRIOS",
     x: 100, y: 940, w: 1000, h: 34, size: 22, weight: .bold, color: purple, align: .left)

box(110, 1030, 440, 205, title: "NotesContext · mcp-notes · Go",
    lines: ["contracts/v1 · schemas e erros", "regras de notas · adaptador MCP", "adaptador SQLite · ledger próprio"],
    fill: white, stroke: purple, titleSize: 21, bodySize: 17)
box(680, 1030, 440, 205, title: "SchedulingContext · mcp-agenda · Go",
    lines: ["contracts/v1 · schemas e erros", "regras de agenda · adaptador MCP", "adaptador SQLite · ledger próprio"],
    fill: white, stroke: purple, titleSize: 20, bodySize: 17)
box(1250, 1030, 420, 205, title: "Novo domínio · MCP Spring",
    lines: ["FUTURO · domínio a definir", "contrato e regras próprios", "cadastro por perfil de integração"],
    fill: futureFill, stroke: futureLine, titleSize: 21, bodySize: 17)

box(140, 1290, 380, 105, title: "SQLite Notes",
    lines: ["Notas + notes_operations"], fill: dbFill, titleSize: 21, bodySize: 16)
box(710, 1290, 380, 105, title: "SQLite Agenda",
    lines: ["Eventos + agenda_operations"], fill: dbFill, titleSize: 21, bodySize: 16)
box(1280, 1290, 360, 105, title: "Persistência própria",
    lines: ["definida pelo novo domínio"], fill: futureFill, stroke: futureLine, titleSize: 20, bodySize: 16)

// Voice and inference paths.
arrow([(535, 280), (535, 305), (60, 305), (60, 532), (105, 532)], both: true)
pill("PCM16 + eventos JSON", x: 310, y: 292, w: 220)
arrow([(525, 715), (605, 715)], both: true)
pill("AgentSession v1alpha1", x: 490, y: 675, w: 165)
arrow([(785, 595), (785, 645)], both: true)
arrow([(965, 715), (1018, 715), (1018, 555), (1080, 555)], both: true)
pill("Realtime + MCP governado", x: 890, y: 654, w: 225)
arrow([(1395, 470), (1395, 285)], both: true)
pill("stream AWS autenticado", x: 1280, y: 345, w: 230)
arrow([(1220, 640), (1220, 700)])
pill("spend metadata", x: 1138, y: 660, w: 165)

// MCP paths. Current apps use stdio; the Spring app is an explicit future extension.
arrow([(1080, 585), (1020, 585), (1020, 990), (330, 990), (330, 1030)], both: true, color: purple)
arrow([(1020, 990), (900, 990), (900, 1030)], both: true, color: purple)
pill("JSON-RPC stdio · contrato da aplicação", x: 455, y: 974, w: 335, color: purple)
arrow([(1020, 990), (1460, 990), (1460, 1030)], both: true, color: futureLine, dashed: true)
pill("transporte futuro", x: 1260, y: 974, w: 170, color: futureLine)
arrow([(330, 1235), (330, 1290)], both: true)
arrow([(900, 1235), (900, 1290)], both: true)
arrow([(1460, 1235), (1460, 1290)], both: true, color: futureLine, dashed: true)

// Footer.
text("Cada aplicação possui seus contratos, regras e dados; o gateway possui Voice v1 e o agente possui prompt e perfis.",
     x: 80, y: 1570, w: 1640, h: 28, size: 20, weight: .semibold, align: .left)
text("LiteLLM é plataforma: concentra credenciais AWS, grants e Usage/Spend sem conteúdo da conversa.",
     x: 80, y: 1605, w: 1640, h: 28, size: 18, align: .left)

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
