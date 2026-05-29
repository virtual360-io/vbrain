require "yaml"
require "fileutils"
require_relative "../paths"
require_relative "../page"

module VBrain
  module Realtime
    module Slack
      SOURCE = "slack".freeze
      SLUG   = "slack".freeze
      TITLE  = "Slack (realtime)".freeze
      TAGS   = %w[slack chat mensagem realtime].freeze

      KEYWORDS = [
        "slack", "canal", "canais", "channel", "channels", "mensagem",
        "mensagens", "message", "messages", "conversa", "conversas",
        "conversation", "thread", "threads", "dm", "dms", "direct message",
        "mensagem direta", "huddle", "workspace", "time", "equipe", "team",
        "menção", "mencao", "menções", "mencionado", "mention", "mentioned",
        "respondeu", "respondi", "responder", "reply", "post", "postou",
        "postei", "escreveu", "escrevi", "falou", "disse", "comentou",
        "anexo", "anexos", "arquivo", "arquivos", "file", "files",
        "quem disse", "alguém falou", "alguem falou"
      ].freeze

      def self.config_path
        File.join(Paths.data_home, "config", "realtime", "slack.yml")
      end

      def self.wiki_path
        File.join(Paths.wiki_dir, "_realtime", "#{SLUG}.md")
      end

      # channels pode ser vazio: lista vazia = busca global no workspace inteiro
      # (todos os canais/DMs acessíveis). Lista preenchida = busca filtrada por
      # canal. Diferente de gmail/gcalendar, aqui o vazio é válido e intencional.
      def self.save_config!(channels:)
        normalized = Array(channels).map { |c| normalize_channel(c) }.reject { |c| c["id"].to_s.empty? && c["name"].to_s.empty? }

        FileUtils.mkdir_p(File.dirname(config_path))
        File.write(config_path, YAML.dump("channels" => normalized))
        normalized
      end

      def self.load_config
        return nil unless File.exist?(config_path)

        data = YAML.safe_load(File.read(config_path), permitted_classes: [Symbol]) || {}
        Array(data["channels"])
      end

      # true quando nenhum canal foi conectado: o handler busca no workspace todo.
      def self.global?(channels)
        Array(channels).reject { |c| normalize_channel(c)["id"].to_s.empty? && normalize_channel(c)["name"].to_s.empty? }.empty?
      end

      def self.write_wiki_page!(channels:)
        FileUtils.mkdir_p(File.dirname(wiki_path))
        Page.write(
          dir:  File.dirname(wiki_path),
          slug: SLUG,
          frontmatter: frontmatter(channels),
          body: body(channels)
        )
      end

      def self.frontmatter(channels)
        {
          "title"    => TITLE,
          "kind"     => "realtime",
          "source"   => SOURCE,
          "tags"     => TAGS,
          "channels" => Array(channels).map { |c| normalize_channel(c) }
        }
      end

      def self.body(channels)
        if global?(channels)
          scope = <<~SCOPE
            Nenhum canal específico conectado: a busca é **global** no workspace
            inteiro (todos os canais/DMs acessíveis), sem filtro `in:`.
          SCOPE
        else
          list = Array(channels).map { |c| "- #{format_channel(c)}" }.join("\n")
          scope = <<~SCOPE
            Canais conectados — a busca filtra por eles (uma chamada por canal,
            já que o Slack search não tem operador `OR`):

            #{list}
          SCOPE
        end

        <<~MD
          # #{TITLE}

          Esta página é uma **fonte realtime**: quando o `/vbrain-query-knowledge`
          a recebe como resultado FTS5, o agente NÃO devolve este body — em vez
          disso chama `mcp__claude_ai_Slack__slack_search_public_and_private`
          com a query do usuário convertida pra Slack search syntax.

          ## Escopo

          #{scope}
          ## Keywords (pra casar no FTS5)

          #{KEYWORDS.join(", ")}.
        MD
      end

      def self.normalize_channel(c)
        h = c.respond_to?(:to_h) ? c.to_h : c
        h = h.transform_keys(&:to_s) if h.is_a?(Hash)
        {
          "id"   => (h["id"] || h["channel_id"]).to_s,
          "name" => h["name"].to_s
        }
      end

      def self.format_channel(c)
        n = normalize_channel(c)
        id   = n["id"]
        name = n["name"]
        return "`#{id}`" if name.empty?
        return "##{name}" if id.empty?

        "##{name} (`#{id}`)"
      end

      # Modificador de canal pra Slack search syntax. Prefere o ID (`in:<#C123>`,
      # robusto a renomeação); cai pro nome (`in:#name`) quando não há ID.
      def self.channel_filter(channel)
        n = normalize_channel(channel)
        return "in:<##{n['id']}>" unless n["id"].empty?
        return "in:##{n['name']}" unless n["name"].empty?

        ""
      end
    end
  end
end
