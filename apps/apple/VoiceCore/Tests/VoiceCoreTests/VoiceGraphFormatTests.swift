import AVFoundation
import XCTest
@testable import VoiceCore

final class VoiceGraphFormatTests: XCTestCase {
    func testDuplexUsesTheExactMicrophoneFormatNotThe24kTransport() throws {
        for rate in [44100.0, 48000.0] {
            let mic = try XCTUnwrap(AVAudioFormat(standardFormatWithSampleRate: rate, channels: 1))
            let speaker = try XCTUnwrap(AVAudioFormat(standardFormatWithSampleRate: 24000, channels: 2))
            let selected = try VoiceGraphFormat.output(microphone: mic, hardware: speaker)
            XCTAssertEqual(selected, mic)
            XCTAssertEqual(selected.sampleRate, rate)
            XCTAssertEqual(selected.channelCount, 1)
        }
    }
    func testPlaybackOnlyUsesHardwareFormat() throws {
        let hardware = try XCTUnwrap(AVAudioFormat(standardFormatWithSampleRate: 48000, channels: 2))
        XCTAssertEqual(try VoiceGraphFormat.output(microphone: nil, hardware: hardware), hardware)
    }
}
