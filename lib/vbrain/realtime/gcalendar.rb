require "yaml"
require "fileutils"
require_relative "../paths"
require_relative "../page"

module VBrain
  module Realtime
    module Gcalendar
      SOURCE = "gcalendar".freeze
      SLUG   = "gcalendar".freeze
      TITLE  = "Google Calendar (realtime)".freeze
      TAGS   = %w[agenda calendar gcalendar realtime].freeze

      KEYWORDS = [
        "agenda", "agendas", "calendário", "calendario", "calendar", "gcalendar",
        "google calendar", "reunião", "reuniões", "reuniao", "reunioes",
        "meeting", "meetings", "evento", "eventos", "event", "events",
        "compromisso", "compromissos", "appointment", "appointments",
        "hoje", "amanhã", "amanha", "ontem", "today", "tomorrow", "yesterday",
        "essa semana", "esta semana", "semana", "próxima semana", "proxima semana",
        "this week", "next week", "mês", "mes", "month", "próximo mês",
        "fim de semana", "weekend", "livre", "ocupado", "disponível", "disponivel",
        "free", "busy", "schedule", "agenda do dia", "rotina"
      ].freeze

      def self.config_path
        File.join(Paths.data_home, "config", "realtime", "gcalendar.yml")
      end

      def self.wiki_path
        File.join(Paths.wiki_dir, "_realtime", "#{SLUG}.md")
      end

      def self.save_config!(calendars:)
        normalized = Array(calendars).map { |c| normalize_calendar(c) }.reject { |c| c["id"].to_s.empty? }
        raise ArgumentError, "at least one calendar required" if normalized.empty?

        FileUtils.mkdir_p(File.dirname(config_path))
        File.write(config_path, YAML.dump("calendars" => normalized))
        normalized
      end

      def self.load_config
        return nil unless File.exist?(config_path)

        data = YAML.safe_load(File.read(config_path), permitted_classes: [Symbol]) || {}
        Array(data["calendars"])
      end

      def self.write_wiki_page!(calendars:)
        FileUtils.mkdir_p(File.dirname(wiki_path))
        Page.write(
          dir:  File.dirname(wiki_path),
          slug: SLUG,
          frontmatter: frontmatter(calendars),
          body: body(calendars)
        )
      end

      def self.frontmatter(calendars)
        {
          "title"     => TITLE,
          "kind"      => "realtime",
          "source"    => SOURCE,
          "tags"      => TAGS,
          "calendars" => Array(calendars).map { |c| normalize_calendar(c) }
        }
      end

      def self.body(calendars)
        list = Array(calendars).map { |c| "- #{format_calendar(c)}" }.join("\n")

        <<~MD
          # #{TITLE}

          Esta página é uma **fonte realtime**: quando o `/vbrain-query-knowledge`
          a recebe como resultado FTS5, o agente NÃO devolve este body — em vez
          disso chama `mcp__claude_ai_Google_Calendar__list_events` com os
          calendários listados abaixo e o intervalo de tempo derivado da query
          (hoje, amanhã, próxima semana, etc).

          ## Calendários conectados

          #{list}

          ## Keywords (pra casar no FTS5)

          #{KEYWORDS.join(", ")}.
        MD
      end

      def self.normalize_calendar(c)
        h = c.respond_to?(:to_h) ? c.to_h : c
        h = h.transform_keys(&:to_s) if h.is_a?(Hash)
        {
          "id"       => h["id"].to_s,
          "summary"  => h["summary"].to_s,
          "timezone" => h["timezone"].to_s
        }
      end

      def self.format_calendar(c)
        id      = c["id"].to_s
        summary = c["summary"].to_s
        return "`#{id}`" if summary.empty? || summary == id

        "#{summary} (`#{id}`)"
      end
    end
  end
end
