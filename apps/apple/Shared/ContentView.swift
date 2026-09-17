import SwiftUI

struct ContentView: View {
    @StateObject private var model = ConversationModel()
    @Environment(\.scenePhase) private var scenePhase
    private let accent = Color(red: 0.28, green: 0.85, blue: 0.83)
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                HStack(alignment: .top) {
                    VStack(alignment: .leading, spacing: 6) {
                        Text("Nova Voice").font(.largeTitle.bold())
                        Text("Voz + ferramentas · PT-BR").font(.subheadline).foregroundStyle(.secondary)
                    }
                    Spacer()
                    Text(model.provider == "fake" ? "FAKE" : "NOVA 2 SONIC")
                        .font(.caption.bold()).padding(8).background(accent.opacity(0.14), in: Capsule())
                }
                VStack(spacing: 18) {
                    Image(systemName: model.playing ? "waveform" : "mic.fill")
                        .font(.system(size: 48)).foregroundStyle(accent)
                        .frame(width: 104, height: 104).background(accent.opacity(0.1), in: Circle())
                    Label(model.stateLabel, systemImage: model.active ? "circle.fill" : "circle")
                        .font(.subheadline).foregroundStyle(model.active ? accent : .gray)
                        .accessibilityIdentifier("conversation.state")
                    if model.provider == "nova" {
                        VStack(alignment: .leading, spacing: 6) {
                            ProgressView(value: min(1, model.inputLevel * 10))
                                .accessibilityLabel("Nível do microfone")
                            Text("Captura: \(model.capturedBuffers) buffers · \(model.capturedBytes) bytes · Enviados: \(model.sentFrames) frames")
                                .font(.caption.monospacedDigit()).foregroundStyle(.secondary)
                            if let warning = model.audioWarning {
                                Text(warning).font(.caption).foregroundStyle(.orange)
                            }
                        }
                    }
                    Button {
                        if model.active { model.stop() } else { model.start() }
                    } label: {
                        Label(model.active ? "Encerrar conversa" : "Iniciar conversa", systemImage: model.active ? "stop.fill" : "mic.fill")
                            .font(.headline).frame(maxWidth: .infinity).padding(.vertical, 10)
                    }.buttonStyle(.borderedProminent).tint(accent).foregroundStyle(.black)
                        .disabled(model.stopping).accessibilityIdentifier("conversation.toggle")
                    Text(model.provider == "fake" ? "Simulação com tom de áudio. Não usa microfone, ASR nem AWS." : "Toque para iniciar e fale naturalmente. Você pode interromper a resposta falando novamente.")
                        .font(.footnote).foregroundStyle(.secondary).multilineTextAlignment(.center)
                }.frame(maxWidth: .infinity).padding(24).background(.white.opacity(0.04), in: RoundedRectangle(cornerRadius: 24))
                if let error = model.errorMessage {
                    Label(error, systemImage: "exclamationmark.triangle.fill").font(.callout)
                        .foregroundStyle(.orange).padding(16).frame(maxWidth: .infinity, alignment: .leading)
                        .background(.orange.opacity(0.08), in: RoundedRectangle(cornerRadius: 12))
                        .accessibilityIdentifier("conversation.error")
                }
                DisclosureGroup("Conexão local") {
                    VStack(alignment: .leading, spacing: 12) {
                        TextField("ws://IP-DO-MAC:8080/v1/voice", text: $model.url)
                            .textFieldStyle(.roundedBorder).accessibilityIdentifier("connection.url")
                        SecureField("Token do gateway (não é uma chave AWS)", text: $model.token)
                            .textFieldStyle(.roundedBorder).accessibilityIdentifier("connection.token")
                        Picker("Provider", selection: $model.provider) {
                            Text("Nova 2 Sonic").tag("nova")
                            Text("Fake · teste local").tag("fake")
                        }.pickerStyle(.segmented).accessibilityIdentifier("connection.provider")
                        Text("No iPhone, use o IP do Mac na mesma rede Wi-Fi. O token fica somente nesta execução do app.")
                            .font(.caption).foregroundStyle(.secondary)
                    }.padding(.top, 12).disabled(model.active)
                }
                VStack(alignment: .leading, spacing: 14) {
                    Text("Conversa").font(.title3.bold())
                    if model.conversation.transcripts.isEmpty {
                        Text("Sua transcrição e as respostas aparecerão aqui.").foregroundStyle(.secondary).font(.callout)
                    }
                    ForEach(model.conversation.transcripts) { line in
                        VStack(alignment: .leading, spacing: 6) {
                            Text(line.role == "USER" ? "Você" : (line.stage == "SPECULATIVE" ? "Nova · resposta planejada" : "Nova"))
                                .font(.caption.bold()).foregroundStyle(line.role == "USER" ? accent : .gray)
                            Text(line.text).textSelection(.enabled)
                        }.padding(14).frame(maxWidth: .infinity, alignment: .leading)
                            .background(.white.opacity(line.role == "USER" ? 0.07 : 0.03), in: RoundedRectangle(cornerRadius: 12))
                    }
                }.accessibilityIdentifier("conversation.transcripts")
                if !model.conversation.operations.isEmpty {
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Ferramentas MCP").font(.title3.bold())
                        ForEach(model.conversation.operations) { operation in
                            VStack(alignment: .leading, spacing: 8) {
                                Label(operation.name, systemImage: "wrench.and.screwdriver").font(.headline)
                                Text(operation.status).font(.subheadline).foregroundStyle(operation.needsConfirmation ? .orange : accent)
                                if !operation.arguments.isEmpty {
                                    DisclosureGroup("Argumentos reais enviados pela Nova") {
                                        Text(operation.arguments).font(.caption.monospaced()).textSelection(.enabled)
                                    }
                                }
                                if operation.needsConfirmation { Text("Responda por voz com a confirmação solicitada pelo assistente.").font(.footnote) }
                                DisclosureGroup("Detalhes do resultado") { Text(operation.details).font(.caption.monospaced()).textSelection(.enabled) }
                            }.padding(16).background(.white.opacity(0.04), in: RoundedRectangle(cornerRadius: 12))
                        }
                    }
                }
                Text("Áudio enviado à AWS somente no modo Nova, pelo seu backend. Não há credenciais AWS no aplicativo. Nenhum áudio é gravado em disco.")
                    .font(.caption).foregroundStyle(.secondary)
                if let id = model.requestId {
                    DisclosureGroup("Identificador do pedido") {
                        Text(id).font(.caption.monospaced()).textSelection(.enabled)
                        Text("Em caso de queda durante uma ação, consulte o resultado antes de repetir. Retry pelo terminal deve usar este requestId.").font(.caption).foregroundStyle(.secondary)
                    }
                }
            }.padding(24).frame(maxWidth: 720).frame(maxWidth: .infinity)
        }.background(LinearGradient(colors: [Color(red: 0.04, green: 0.08, blue: 0.14), Color(red: 0.08, green: 0.06, blue: 0.16)], startPoint: .topLeading, endPoint: .bottomTrailing))
            .preferredColorScheme(.dark)
        #if os(macOS)
            .frame(minWidth: 480, minHeight: 640)
        #endif
            .onChange(of: scenePhase) { _, phase in if phase == .background { model.stop() } }
            .onDisappear { model.shutdown() }
    }
}
