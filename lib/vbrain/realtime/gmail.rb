require "yaml"
require "fileutils"
require_relative "../paths"
require_relative "../page"

module VBrain
  module Realtime
    module Gmail
      SOURCE = "gmail".freeze
      SLUG   = "gmail".freeze
      TITLE  = "Gmail (realtime)".freeze
      TAGS   = %w[email gmail inbox realtime].freeze

      KEYWORDS = [
        "email", "emails", "e-mail", "e-mails", "mail", "mensagem", "mensagens",
        "message", "messages", "inbox", "caixa de entrada", "remetente", "sender",
        "enviado", "enviada", "sent", "recebido", "recebida", "received",
        "responder", "respondeu", "respondi", "anexo", "anexos", "attachment",
        "attachments", "gmail", "google mail", "assunto", "subject", "thread",
        "conversa", "conversation", "rascunho", "draft", "spam", "lixeira",
        "trash", "estrelado", "starred", "important", "importante", "não lido",
        "nao lido", "unread", "label", "marcador"
      ].freeze

      def self.config_path
        File.join(Paths.data_home, "config", "realtime", "gmail.yml")
      end

      def self.wiki_path
        File.join(Paths.wiki_dir, "_realtime", "#{SLUG}.md")
      end

      def self.save_config!(labels:)
        normalized = Array(labels).map { |l| normalize_label(l) }.reject { |l| l["id"].to_s.empty? }
        raise ArgumentError, "at least one label required" if normalized.empty?

        FileUtils.mkdir_p(File.dirname(config_path))
        File.write(config_path, YAML.dump("labels" => normalized))
        normalized
      end

      def self.load_config
        return nil unless File.exist?(config_path)

        data = YAML.safe_load(File.read(config_path), permitted_classes: [Symbol]) || {}
        Array(data["labels"])
      end

      def self.write_wiki_page!(labels:)
        FileUtils.mkdir_p(File.dirname(wiki_path))
        Page.write(
          dir:  File.dirname(wiki_path),
          slug: SLUG,
          frontmatter: frontmatter(labels),
          body: body(labels)
        )
      end

      def self.frontmatter(labels)
        {
          "title"  => TITLE,
          "kind"   => "realtime",
          "source" => SOURCE,
          "tags"   => TAGS,
          "labels" => Array(labels).map { |l| normalize_label(l) }
        }
      end

      def self.body(labels)
        list = Array(labels).map { |l| "- #{format_label(l)}" }.join("\n")

        <<~MD
          # #{TITLE}

          Esta página é uma **fonte realtime**: quando o `/vbrain-query-knowledge`
          a recebe como resultado FTS5, o agente NÃO devolve este body — em vez
          disso chama `mcp__claude_ai_Gmail__search_threads` prependendo um
          filtro `(label:<id1> OR label:<id2> …)` com os labels conectados.

          ## Labels conectados

          #{list}

          ## Keywords (pra casar no FTS5)

          #{KEYWORDS.join(", ")}.
        MD
      end

      def self.normalize_label(l)
        h = l.respond_to?(:to_h) ? l.to_h : l
        h = h.transform_keys(&:to_s) if h.is_a?(Hash)
        id = (h["id"] || h["labelId"]).to_s
        {
          "id"   => id,
          "name" => h["name"].to_s
        }
      end

      def self.format_label(l)
        id   = l["id"].to_s
        name = l["name"].to_s
        return "`#{id}`" if name.empty? || name == id

        "#{name} (`#{id}`)"
      end

      def self.label_filter_clause(labels)
        ids = Array(labels).map { |l| l["id"].to_s }.reject(&:empty?)
        return "" if ids.empty?
        return "label:#{ids.first}" if ids.size == 1

        "(" + ids.map { |id| "label:#{id}" }.join(" OR ") + ")"
      end
    end
  end
end
