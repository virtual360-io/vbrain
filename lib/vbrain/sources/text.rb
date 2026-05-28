require "fileutils"
require_relative "base"

module VBrain
  module Sources
    module Text
      extend Base

      EXTENSIONS = %w[.md .markdown .txt .text].freeze
      SAMPLE_BYTES = 4096

      def self.kind_key
        "text"
      end

      def self.detect?(path)
        return false unless File.file?(path)

        ext = File.extname(path).downcase
        return true if EXTENSIONS.include?(ext)

        utf8_text?(path)
      end

      def self.extract(path, out_path)
        FileUtils.mkdir_p(File.dirname(out_path))
        content = File.read(path)
        File.write(out_path, content)
        out_path
      end

      def self.utf8_text?(path)
        sample = File.binread(path, SAMPLE_BYTES)
        return true if sample.empty?
        return false if sample.include?("\x00")

        sample.force_encoding("UTF-8").valid_encoding?
      rescue StandardError
        false
      end
    end
  end
end
