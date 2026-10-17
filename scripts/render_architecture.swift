#!/usr/bin/env swift

import AppKit
import Foundation

let width = 2200
let height = 1800
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
        text(lines.joined(separator: "\n"), x: x + 18, y: y + 62, w: w - 36, h: max(26, h - 70),
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

// Current implementation: two agents, four MCP applications and Cognito login.
text("STS Model — arquitetura atual", x: 80, y: 36, w: 2000, h: 55,
     size: 42, weight: .bold, align: .left)
text("Mac / iOS · voz e ferramentas · identidade e perfil com dados próprios",
     x: 80, y: 100, w: 1950, h: 34, size: 24, align: .left)
let authColor = NSColor(calibratedRed: 0.76, green: 0.44, blue: 0.12, alpha: 1)
let controlColor = NSColor(calibratedRed: 0.12, green: 0.48, blue: 0.42, alpha: 1)

box(90, 175, 650, 170, title: "APP APPLE · SwiftUI · macOS / iOS",
    lines: ["Voz v1/v2 · modal Cognito em WKWebView", "Keychain: sessão + instalação", "CLI Go: cliente de voz v1"], titleSize: 25, bodySize: 21)
box(1010, 175, 500, 170, title: "AWS · Bedrock / Nova 2 Sonic",
    lines: ["Voz carolina · áudio + transcrição", "Saudações e conversa pelo agente"], fill: awsFill,
    stroke: authColor, titleSize: 24, bodySize: 21)
box(1680, 175, 450, 170, title: "AWS · Cognito",
    lines: ["Managed Login · Essentials", "Authorization Code + PKCE", "MFA obrigatório por e-mail"], fill: awsFill,
    stroke: authColor, titleSize: 25, bodySize: 20)
box(1680, 385, 450, 95, title: "SES · e-mail transacional",
    lines: ["Convite, senha temporária e MFA"], fill: awsFill,
    stroke: authColor, titleSize: 22, bodySize: 18)

rounded(rect(50, 515, 2100, 1040), fill: localFill, stroke: border, radius: 28)
text("APLICAÇÕES LOCAIS · MONOREPO POLIGLOTA · PORTAS PADRÃO DE DESENVOLVIMENTO",
     x: 85, y: 533, w: 2000, h: 32, size: 22, weight: .bold, align: .left)
box(90, 590, 440, 280, title: "voice-gateway · Go · :8080",
    lines: ["Transporte /v1/voice e /v2/voice", "Catálogo e roteamento de agentes", "Controle /v2/auth/* + introspecção", "Sem runtime LLM nem cliente MCP"],
    fill: gatewayFill, titleSize: 23, bodySize: 20)
box(600, 590, 300, 280, title: "Agenda / notas",
    lines: ["personal-assistant-agent", "Go · :8090 · AgentSession v1", "Prompt + ferramentas", "Confirmação + idempotência"],
    fill: gatewayFill, titleSize: 24, bodySize: 17)
box(940, 590, 300, 280, title: "Agente Conta",
    lines: ["account-agent · Go · :8091", "AgentSession v2 + A2UI", "Saudações e conversa Nova", "Consentimento + política MCP"],
    fill: gatewayFill, titleSize: 24, bodySize: 17)
box(1340, 590, 740, 280, title: "LiteLLM · plataforma local",
    lines: ["Realtime → Bedrock · MCP → aplicações", "Chaves virtuais e grants por agente / ferramenta", "Credenciais AWS de inferência somente aqui", ":4001 na composição Conta · :4000 na composição legada"],
    fill: dockerFill, stroke: purple, titleSize: 26, bodySize: 22)
box(1700, 915, 380, 100, title: "PostgreSQL · LiteLLM",
    lines: ["Chaves + uso/custo, sem conteúdo"], fill: dbFill, titleSize: 21, bodySize: 17)

box(90, 1080, 440, 215, title: "Notas · mcp-notes · Go",
    lines: ["MCP stdio · contrato v1", "Regras e schemas de notas", "Ledger idempotente próprio"], stroke: purple, titleSize: 24, bodySize: 21)
box(610, 1080, 440, 215, title: "Agenda · mcp-agenda · Go",
    lines: ["MCP stdio · contrato v1", "Eventos e disponibilidade", "Ledger idempotente próprio"], stroke: purple, titleSize: 24, bodySize: 21)
box(1130, 1080, 440, 215, title: "Identidade · mcp-identity",
    lines: ["Java 21 / Spring · HTTP · :8092", "MCP v2 + API interna", "OAuth/PKCE, sessões e revogação", "Validação JWT / nonce / MFA"], stroke: purple, titleSize: 23, bodySize: 20)
box(1650, 1080, 440, 215, title: "Perfil · mcp-profile",
    lines: ["Java 21 / Spring · HTTP · :8093", "MCP v2 · dados do próprio usuário", "Versão, confirmação e ledger", "Valida sessão via identidade"], stroke: purple, titleSize: 24, bodySize: 20)
box(110, 1370, 400, 110, title: "SQLite · Notas",
    lines: ["Notas + operações"], fill: dbFill, titleSize: 23, bodySize: 21)
box(630, 1370, 400, 110, title: "SQLite · Agenda",
    lines: ["Eventos + operações"], fill: dbFill, titleSize: 23, bodySize: 21)
box(1150, 1370, 400, 110, title: "PostgreSQL · Identidade",
    lines: ["Sessões + credenciais protegidas"], fill: dbFill, titleSize: 23, bodySize: 19)
box(1670, 1370, 400, 110, title: "PostgreSQL · Perfil",
    lines: ["Perfil + versão + operações"], fill: dbFill, titleSize: 23, bodySize: 20)

// External browser login and voice paths are distinct.
arrow([(740, 220), (790, 220), (790, 145), (1905, 145), (1905, 175)], both: true, color: authColor)
pill("HTTPS · login e MFA no modal", x: 1030, y: 130, w: 350, color: authColor)
arrow([(1905, 345), (1905, 385)], color: authColor)
arrow([(310, 345), (310, 420), (65, 420), (65, 700), (90, 700)], both: true)
pill("WebSocket · PCM16 + JSON", x: 130, y: 404, w: 330)
arrow([(1480, 590), (1480, 345)], both: true, color: purple)
pill("Realtime autenticado", x: 1340, y: 440, w: 250, color: purple)

// Public voice is translated to each agent's session contract.
arrow([(530, 700), (600, 700)], both: true)
pill("v1", x: 540, y: 663, w: 50)
arrow([(510, 870), (510, 900), (1090, 900), (1090, 870)], both: true)
pill("AgentSession v2 · Conta / A2UI", x: 600, y: 884, w: 380)
arrow([(750, 590), (750, 575), (1710, 575), (1710, 590)], both: true, color: purple)
arrow([(1090, 590), (1090, 575)], both: true, color: purple)
pill("Realtime + MCP governado", x: 1100, y: 560, w: 310, color: purple)
arrow([(1890, 870), (1890, 915)], color: purple)

// Auth control bypasses the conversational tool path.
arrow([(130, 870), (130, 960), (1350, 960), (1350, 1080)], both: true, color: controlColor)
pill("Controle de autenticação · HTTP interno", x: 405, y: 944, w: 500, color: controlColor)

// Tool access always goes through LiteLLM and the agent's grants.
arrow([(1510, 870), (1510, 1025), (310, 1025), (310, 1080)], both: true, color: purple)
arrow([(830, 1025), (830, 1080)], both: true, color: purple)
arrow([(1450, 1025), (1450, 1080)], both: true, color: purple)
arrow([(1510, 1025), (1870, 1025), (1870, 1080)], both: true, color: purple)
pill("MCP por stdio / HTTP · envelopes assinados v1 / v2", x: 455, y: 1009, w: 620, color: purple)
for x: CGFloat in [310, 830, 1350, 1870] {
    arrow([(x, 1295), (x, 1370)], both: true)
}

// Identity's own provider adapter validates/exchanges with Cognito.
arrow([(1570, 1200), (1610, 1200), (1610, 1515), (2175, 1515), (2175, 260), (2130, 260)], both: true, color: authColor)
pill("OAuth / OIDC · tokens / JWKS", x: 1740, y: 1498, w: 370, color: authColor)

text("FLUXO DE CONTA", x: 80, y: 1590, w: 2000, h: 32, size: 23, weight: .bold, align: .left)
text("Saudação Nova → sim / não → modal de login + MFA → sessão validada → confirmação Nova → consultar / editar perfil",
     x: 80, y: 1638, w: 2040, h: 55, size: 25, align: .left)
text("Cada aplicação possui seus dados e regras. Senha e MFA ficam na página Cognito; o modelo não recebe credenciais.",
     x: 80, y: 1710, w: 2040, h: 34, size: 22, weight: .medium, align: .left)
text("Azul: voz / dados locais · Roxo: inferência e ferramentas · Verde: controle de sessão · Laranja: autenticação Cognito",
     x: 80, y: 1750, w: 2040, h: 28, size: 20, align: .left)

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
