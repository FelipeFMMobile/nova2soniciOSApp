import AVFoundation
import AudioToolbox
import XCTest
@testable import VoiceCore

final class PCMConverterTests: XCTestCase {
    private func nineChannelFormat() throws -> AVAudioFormat {
        let layout = try XCTUnwrap(AVAudioChannelLayout(layoutTag: kAudioChannelLayoutTag_DiscreteInOrder | 9))
        return try XCTUnwrap(AVAudioFormat(standardFormatWithSampleRate: 48000, channelLayout: layout))
    }
    func testNineChannelsSelectChannelZeroWithoutCancellation() throws {
        let format = try nineChannelFormat()
        let buffer = try XCTUnwrap(AVAudioPCMBuffer(pcmFormat: format, frameCapacity: 4800))
        buffer.frameLength = 4800
        for channel in 0..<9 {
            for frame in 0..<4800 {
                let voice = 0.25 * sin(Float(frame) * 2 * .pi * 440 / 48000)
                buffer.floatChannelData![channel][frame] = channel == 0 ? voice : (channel == 1 ? -voice : 0)
            }
        }
        let levels = VoiceDiagnostics.channelLevels(buffer)
        XCTAssertEqual(levels.count, 9)
        XCTAssertGreaterThan(levels[0], 0.17)
        XCTAssertEqual(levels[0], levels[1], accuracy: 0.0001)
        XCTAssertEqual(levels[8], 0)
        let pcm = try PCMConverter(input: format).convert(buffer)
        XCTAssertGreaterThan(VoiceDiagnostics.level(pcm), 0.16)
        XCTAssertLessThan(VoiceDiagnostics.level(pcm), 0.19)
    }

    func testNineChannelsDoNotSelectAuxiliarySignalWhenMicIsSilent() throws {
        let format = try nineChannelFormat()
        let buffer = try XCTUnwrap(AVAudioPCMBuffer(pcmFormat: format, frameCapacity: 4800))
        buffer.frameLength = 4800
        for channel in 0..<9 {
            for frame in 0..<4800 { buffer.floatChannelData![channel][frame] = channel == 8 ? 0.5 : 0 }
        }
        let pcm = try PCMConverter(input: format).convert(buffer)
        XCTAssertEqual(VoiceDiagnostics.level(pcm), 0)
    }

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
