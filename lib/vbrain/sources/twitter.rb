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
      USER_AGENT = "vbrain/1.0"
      BROWSER_UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"
      TIMEOUT = 10
      PLAYWRIGHT_TIMEOUT_MS = 30_000
      STEALTH_INIT_SCRIPT = <<~JS.freeze
        Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
        Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
        Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });
      JS

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
        parsed = JSON.parse(json)
        article_full = nil
        if parsed["article"]
          article_full = fetch_article_via_playwright("https://x.com/i/status/#{id}?s=20")
        end
        md = extract_from_json(json, url: url, id: id, article_full_text: article_full)
        FileUtils.mkdir_p(File.dirname(out_path))
        File.write(out_path, md)
        out_path
      end

      def self.playwright_available?
        return false unless system("command -v playwright > /dev/null 2>&1")

        begin
          require "playwright"
          true
        rescue LoadError
          false
        end
      end

      def self.fetch_article_via_playwright(tweet_url)
        return nil unless playwright_available?

        require "playwright"
        text = nil
        Playwright.create(playwright_cli_executable_path: "playwright") do |pw|
          browser = pw.chromium.launch(
            channel: "chrome",
            headless: true,
            args: ["--disable-blink-features=AutomationControlled", "--no-sandbox"]
          )
          context = browser.new_context(
            userAgent: BROWSER_UA,
            viewport: { width: 1280, height: 800 },
            locale: "en-US"
          )
          context.add_init_script(script: STEALTH_INIT_SCRIPT)
          page = context.new_page
          page.goto(tweet_url, waitUntil: "domcontentloaded", timeout: PLAYWRIGHT_TIMEOUT_MS)
          begin
            page.wait_for_selector("article", timeout: 15_000)
          rescue StandardError
            # selector may not appear; fall through to body grab
          end
          sleep 3
          text = page.evaluate("document.body.innerText").to_s
          browser.close
        end
        text
      rescue StandardError => e
        warn "twitter article playwright fetch failed: #{e.class}: #{e.message}"
        nil
      end

      def self.extract_from_json(json_str, url:, id:, article_full_text: nil)
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
          lines << "## Artigo embutido"
          lines << ""
          lines << "- Artigo título: #{article['title'].to_s.strip}" if article["title"]
          lines << "- Artigo ID: #{article['rest_id']}" if article["rest_id"]
          lines << ""
          if article_full_text && article_full_text.to_s.length > 500
            cleaned = clean_article_text(article_full_text, title: article["title"])
            lines << "**Body completo** (extraído via Playwright + Chrome do sistema):"
            lines << ""
            lines << "```"
            lines << cleaned
            lines << "```"
            lines << ""
          else
            lines << "**Nota**: o body completo do artigo só é acessível com auth no X ou via Playwright/Chrome real. O texto abaixo é o `preview_text` (~200 chars) entregue pelo syndication público — use como excerpt literal, não infira o resto."
            lines << ""
            if article["preview_text"]
              lines << "```"
              lines << article["preview_text"].to_s
              lines << "```"
              lines << ""
            end
          end
        end

        lines.join("\n") + "\n"
      end

      def self.clean_article_text(raw_text, title: nil)
        text = raw_text.to_s
        if title && (idx = text.index(title.to_s.strip))
          text = text[idx..]
        end
        boilerplate_markers = [
          "About\n |\nDownload the X app",
          "© 2026 X Corp.",
          "Don’t miss what",
          "People on X are the first",
          "Log in\nSign up"
        ]
        boilerplate_markers.each do |marker|
          if (cut = text.index(marker))
            text = text[0...cut]
          end
        end
        text.gsub(/\n{3,}/, "\n\n").strip
      end
    end
  end
end
