require "net/http"
require "uri"
require "digest"
require "fileutils"
require "time"
require_relative "base"
require_relative "../slug"

module VBrain
  module Sources
    module Url
      extend Base

      URL_RE = %r{\Ahttps?://}i
      JINA_BASE = "https://r.jina.ai"
      USER_AGENT = "vbrain/1.0"
      TIMEOUT = 30

      class FetchError < StandardError; end

      def self.kind_key
        "url"
      end

      def self.detect?(input)
        input.to_s.match?(URL_RE)
      end

      def self.copy_to_raw(url, raw_dir, timestamp)
        markdown = fetch_jina(url)
        host = (URI(url).host || "url").to_s
        host_slug = VBrain::Slug.from(host)
        basename = "#{timestamp}-#{host_slug}.md"
        dest = File.join(raw_dir, basename)
        FileUtils.mkdir_p(File.dirname(dest))
        File.write(dest, markdown)
        {
          "path" => dest,
          "original_filename" => basename,
          "sha256" => Digest::SHA256.hexdigest("#{url}\n#{markdown}"),
          "markdown" => markdown
        }
      end

      def self.extract(url, out_path, raw_info: {})
        md = raw_info["markdown"] || fetch_jina(url)
        FileUtils.mkdir_p(File.dirname(out_path))
        File.write(out_path, md)
        out_path
      end

      def self.fetch_jina(url)
        target = "#{JINA_BASE}/#{url}"
        uri = URI(target)
        req = Net::HTTP::Get.new(uri.request_uri)
        req["Accept"] = "text/markdown"
        req["User-Agent"] = USER_AGENT
        token = ENV["JINA_API_KEY"]
        req["Authorization"] = "Bearer #{token}" if token && !token.empty?

        res = Net::HTTP.start(uri.hostname, uri.port,
                              use_ssl: uri.scheme == "https",
                              read_timeout: TIMEOUT, open_timeout: TIMEOUT) do |http|
          http.request(req)
        end

        unless res.is_a?(Net::HTTPSuccess)
          raise FetchError, "jina HTTP #{res.code} for #{url}: #{res.body.to_s[0, 300]}"
        end

        body = res.body.to_s
        raise FetchError, "jina empty body for #{url}" if body.strip.empty?

        body
      end
    end
  end
end
