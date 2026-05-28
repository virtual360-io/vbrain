require "net/http"
require "uri"
require "digest"
require "fileutils"
require "time"
require "nokogiri"
require_relative "base"
require_relative "../slug"

module VBrain
  module Sources
    module Url
      extend Base

      URL_RE = %r{\Ahttps?://}i
      USER_AGENT = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " \
                   "(KHTML, like Gecko) Chrome/121.0 Safari/537.36"
      MAX_REDIRECTS = 5
      TIMEOUT = 15

      class FetchError < StandardError; end

      def self.kind_key
        "url"
      end

      def self.detect?(input)
        input.to_s.match?(URL_RE)
      end

      def self.copy_to_raw(url, raw_dir, timestamp)
        html, final_url, status = fetch(url)
        host_slug = VBrain::Slug.from(URI(final_url).host || "url")
        basename = "#{timestamp}-#{host_slug}.html"
        dest = File.join(raw_dir, basename)
        FileUtils.mkdir_p(File.dirname(dest))
        File.write(dest, html)
        {
          "path" => dest,
          "original_filename" => basename,
          "sha256" => Digest::SHA256.hexdigest("#{final_url}\n#{html}"),
          "final_url" => final_url,
          "http_status" => status,
          "html" => html
        }
      end

      def self.extract(url, out_path, raw_info: {})
        html = raw_info["html"]
        final_url = raw_info["final_url"]
        if html.nil?
          html, final_url, _ = fetch(url)
        end

        markdown = extract_from_html(html, url: url, final_url: final_url)
        FileUtils.mkdir_p(File.dirname(out_path))
        File.write(out_path, markdown)
        out_path
      end

      def self.extract_from_html(html, url:, final_url: nil)
        final_url ||= url
        doc = Nokogiri::HTML(html)

        og_title = doc.at_css('meta[property="og:title"]')&.attr("content")
        og_desc  = doc.at_css('meta[property="og:description"]')&.attr("content")
        og_site  = doc.at_css('meta[property="og:site_name"]')&.attr("content")
        title    = og_title || doc.at_css("title")&.text&.strip

        doc.css("script, style, noscript, template").each(&:remove)
        main = doc.at_css("article") || doc.at_css("main") || doc.at_css("body") || doc
        text = main.text.to_s.gsub(/[ \t]+/, " ").gsub(/\n{3,}/, "\n\n").strip
        text_limit = 8_000
        text = "#{text[0, text_limit]}\n\n…(truncado em #{text_limit} chars)" if text.length > text_limit

        lines = []
        lines << "# #{title || final_url}"
        lines << ""
        lines << "- Source URL: #{url}"
        lines << "- Final URL: #{final_url}" if final_url != url
        lines << "- Site: #{og_site}" if og_site
        lines << "- Fetched at: #{Time.now.utc.iso8601}"
        lines << ""
        if og_desc && !og_desc.empty?
          lines << "## Resumo (Open Graph)"
          lines << ""
          lines << og_desc
          lines << ""
        end
        lines << "## Conteúdo extraído"
        lines << ""
        lines << (text.empty? ? "(sem conteúdo textual extraível — possível login wall ou render JS)" : text)
        lines.join("\n") + "\n"
      end

      def self.fetch(url, redirects_left: MAX_REDIRECTS)
        uri = URI(url)
        raise FetchError, "unsupported scheme: #{uri.scheme}" unless %w[http https].include?(uri.scheme)

        req = Net::HTTP::Get.new(uri.request_uri)
        req["User-Agent"] = USER_AGENT
        req["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
        req["Accept-Language"] = "en-US,en;q=0.9,pt-BR;q=0.8"

        res = Net::HTTP.start(uri.hostname, uri.port,
                              use_ssl: uri.scheme == "https",
                              read_timeout: TIMEOUT, open_timeout: TIMEOUT) do |http|
          http.request(req)
        end

        case res
        when Net::HTTPRedirection
          raise FetchError, "too many redirects" if redirects_left <= 0

          target = URI.join(url, res["location"]).to_s
          return fetch(target, redirects_left: redirects_left - 1)
        when Net::HTTPSuccess
          [res.body.to_s, uri.to_s, res.code.to_i]
        else
          [res.body.to_s, uri.to_s, res.code.to_i]
        end
      end
    end
  end
end
