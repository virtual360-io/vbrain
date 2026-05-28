require "net/http"
require "uri"
require "json"
require "digest"
require "fileutils"
require "time"
require_relative "base"

module VBrain
  module Sources
    module Twitter
      extend Base

      TWEET_URL_RE = %r{
        \A
        (?:https?://)?
        (?:www\.|m\.|mobile\.)?
        (?:twitter\.com|x\.com)
        /(?<user>[A-Za-z0-9_]+)
        /status/(?<id>\d+)
      }ix

      SYNDICATION_URL = "https://cdn.syndication.twimg.com/tweet-result"
      USER_AGENT = "Mozilla/5.0 (compatible; vbrain/1.0; +https://github.com/akitaonrails/ai-memory)"
      TIMEOUT = 10

      class FetchError < StandardError; end

      def self.kind_key
        "tweet"
      end

      def self.detect?(input)
        !!input.to_s.match(TWEET_URL_RE)
      end

      def self.parse_id(url)
        m = url.to_s.match(TWEET_URL_RE) or raise FetchError, "not a tweet URL: #{url}"
        m[:id]
      end

      def self.compute_token(id)
        n = id.to_i.to_f / 1e15 * Math::PI
        n.to_s.gsub(/0+\z/, "").gsub(".", "")
      end

      def self.fetch_syndication(id)
        token = compute_token(id)
        uri = URI("#{SYNDICATION_URL}?id=#{id}&lang=en&token=#{token}")
        req = Net::HTTP::Get.new(uri.request_uri)
        req["User-Agent"] = USER_AGENT
        req["Accept"] = "application/json"
        res = Net::HTTP.start(uri.hostname, uri.port,
                              use_ssl: true, read_timeout: TIMEOUT, open_timeout: TIMEOUT) do |http|
          http.request(req)
        end
        raise FetchError, "syndication HTTP #{res.code}" unless res.is_a?(Net::HTTPSuccess)

        res.body.to_s
      end

      def self.copy_to_raw(url, raw_dir, timestamp)
        id = parse_id(url)
        json = fetch_syndication(id)
        basename = "#{timestamp}-tweet-#{id}.json"
        dest = File.join(raw_dir, basename)
        FileUtils.mkdir_p(File.dirname(dest))
        File.write(dest, json)
        {
          "path" => dest,
          "original_filename" => basename,
          "sha256" => Digest::SHA256.hexdigest("#{url}\n#{json}"),
          "tweet_id" => id,
          "json" => json
        }
      end

      def self.extract(url, out_path, raw_info: {})
        id = raw_info["tweet_id"] || parse_id(url)
        json = raw_info["json"] || fetch_syndication(id)
        md = extract_from_json(json, url: url, id: id)
        FileUtils.mkdir_p(File.dirname(out_path))
        File.write(out_path, md)
        out_path
      end

      def self.extract_from_json(json_str, url:, id:)
        data = JSON.parse(json_str)
        user       = data["user"] || {}
        handle     = user["screen_name"]
        name       = user["name"]
        created_at = data["created_at"]
        text       = data["text"].to_s
        urls       = (data.dig("entities", "urls") || []).map do |u|
          { "display" => u["display_url"], "expanded" => u["expanded_url"], "shortened" => u["url"] }
        end
        media      = (data["mediaDetails"] || []).map do |m|
          { "type" => m["type"], "url" => m["media_url_https"] || m["media_url"] }
        end
        favorites  = data["favorite_count"]
        lang       = data["lang"]
        article    = data["article"]

        text_expanded = urls.reduce(text) do |acc, u|
          u["shortened"] && u["expanded"] ? acc.gsub(u["shortened"], u["expanded"]) : acc
        end

        title = "Tweet de #{name || handle} (#{created_at})"

        lines = []
        lines << "# #{title}"
        lines << ""
        lines << "- Source URL: #{url}"
        lines << "- Tweet ID: #{id}"
        lines << "- Autor: #{name} (@#{handle})" if handle
        lines << "- Data: #{created_at}" if created_at
        lines << "- Idioma: #{lang}" if lang
        lines << "- Likes (no momento da ingestão): #{favorites}" if favorites
        lines << ""
        lines << "## Texto do tweet"
        lines << ""
        if text_expanded.strip.empty?
          lines << "(tweet sem texto — apenas mídia ou link)"
        else
          lines << text_expanded.strip
        end
        lines << ""

        unless urls.empty?
          lines << "## Links citados"
          lines << ""
          urls.each do |u|
            lines << "- [#{u['display']}](#{u['expanded']})"
          end
          lines << ""
        end

        unless media.empty?
          lines << "## Mídia"
          lines << ""
          media.each do |m|
            lines << "- #{m['type']}: #{m['url']}"
          end
          lines << ""
        end

        if article && (article["title"] || article["preview_text"])
          lines << "## Artigo embutido (preview do syndication)"
          lines << ""
          lines << "- Artigo título: #{article['title'].to_s.strip}" if article["title"]
          lines << "- Artigo ID: #{article['rest_id']}" if article["rest_id"]
          lines << ""
          lines << "**Nota**: o body completo do artigo só é acessível com auth no X. O texto abaixo é o `preview_text` (~200 chars) entregue pelo syndication público — use como excerpt literal, não infira o resto."
          lines << ""
          if article["preview_text"]
            lines << "```"
            lines << article["preview_text"].to_s
            lines << "```"
            lines << ""
          end
        end

        lines.join("\n") + "\n"
      end
    end
  end
end
