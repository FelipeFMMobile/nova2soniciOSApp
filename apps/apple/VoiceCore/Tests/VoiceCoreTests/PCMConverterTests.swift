import AVFoundation
import XCTest
@testable import VoiceCore

final class PCMConverterTests: XCTestCase {
    func testStereo48kBecomesBoundedMono16kPCM16() throws {
        let format = try XCTUnwrap(AVAudioFormat(standardFormatWithSampleRate: 48000, channels: 2))
        let buffer = try XCTUnwrap(AVAudioPCMBuffer(pcmFormat: format, frameCapacity: 1536))
        buffer.frameLength = 1536
        for channel in 0..<2 {
            for i in 0..<1536 { buffer.floatChannelData![channel][i] = 0.25 * sin(Float(i) * 2 * .pi * 440 / 48000) }
        }
        let converter = try PCMConverter(input: format)
        let pcm = try converter.convert(buffer)
        XCTAssertGreaterThan(pcm.count, 800); XCTAssertLessThanOrEqual(pcm.count, 1152)
        XCTAssertEqual(pcm.count % 2, 0); XCTAssertTrue(pcm.contains { $0 != 0 })
        let next = try converter.convert(buffer)
        XCTAssertEqual(next.count, 1024)
    }
    func testDifferentInputFormatIsRejected() throws {
        let converter = try PCMConverter(input: XCTUnwrap(AVAudioFormat(standardFormatWithSampleRate: 48000, channels: 2)))
        let buffer = try XCTUnwrap(AVAudioPCMBuffer(pcmFormat: XCTUnwrap(AVAudioFormat(standardFormatWithSampleRate: 44100, channels: 1)), frameCapacity: 100))
        XCTAssertThrowsError(try converter.convert(buffer))
    }
}
