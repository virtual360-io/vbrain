require "digest"
require "fileutils"

module VBrain
  module Sources
    module Base
      def detect?(_input)
        raise NotImplementedError
      end

      def kind_key
        raise NotImplementedError
      end

      def extract(_input, _out_path, raw_info: {})
        raise NotImplementedError
      end

      # Sources whose input is a local file get this default. Sources whose
      # input is something else (URL, remote handle) override.
      def copy_to_raw(input, raw_dir, timestamp)
        basename = File.basename(input)
        dest = File.join(raw_dir, "#{timestamp}-#{basename}")
        FileUtils.cp(input, dest)
        {
          "path" => dest,
          "original_filename" => basename,
          "sha256" => Digest::SHA256.file(input).hexdigest
        }
      end
    end
  end
end
