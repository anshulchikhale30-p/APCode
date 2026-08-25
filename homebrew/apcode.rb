# Homebrew Formula for APCode
# This is a template. GoReleaser will publish the real formula to anshulchikhale30-p/homebrew-tap on release.
# For local tap testing: `brew install --build-from-source ./homebrew/apcode.rb`
class Apcode < Formula
  desc "APCode — offline-first AI coding agent that adapts to your laptop"
  homepage "https://github.com/anshulchikhale30-p/APCode"
  version "0.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/anshulchikhale30-p/APCode/releases/download/v0.1.0/apcode_0.1.0_darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_SHA256_DARWIN_ARM64"
    else
      url "https://github.com/anshulchikhale30-p/APCode/releases/download/v0.1.0/apcode_0.1.0_darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_SHA256_DARWIN_AMD64"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/anshulchikhale30-p/APCode/releases/download/v0.1.0/apcode_0.1.0_linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_SHA256_LINUX_ARM64"
    else
      url "https://github.com/anshulchikhale30-p/APCode/releases/download/v0.1.0/apcode_0.1.0_linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_SHA256_LINUX_AMD64"
    end
  end

  def install
    bin.install "apcode"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/apcode --version")
    system bin/"apcode", "--help"
  end

  caveats do
    <<~EOS
      APCode is offline-first. Everything runs locally — no cloud APIs.

      Try:
        apcode --help
        apcode benchmark
        apcode models
        apcode recommend --benchmark

      Docs: https://github.com/anshulchikhale30-p/APCode#readme
    EOS
  end
end
