require "fileutils"
require_relative "paths"

module VBrain
  # Instala os "assets do agente" no repo da base (~/vbrain) pra ela ser
  # autossuficiente — rodar em qualquer ambiente que a clone (ex.: cloud da
  # Anthropic), sem depender do repo de código separado:
  #   - CLAUDE.md instruindo o agente a SEMPRE usar as skills;
  #   - cópia versionada das skills em .claude/skills/ (cruas, paths relativos);
  #   - cópia do código que as skills invocam: scripts/, lib/, Gemfile,
  #     Gemfile.lock, .ruby-version.
  # O código canônico continua no repo do projeto; aqui é uma cópia sincronizada
  # por scripts/install.rb (rerode após git pull).
  module Scaffold
    SRC_ROOT   = Paths::PROJECT_ROOT
    SKILLS_SRC = File.join(SRC_ROOT, ".claude", "skills").freeze

    # Código copiado pra raiz da base (as skills cruas chamam `scripts/...` por
    # caminho relativo, então precisam rodar do root da base).
    CODE_DIRS  = %w[scripts lib].freeze
    CODE_FILES = %w[Gemfile Gemfile.lock .ruby-version].freeze

    CLAUDE_MD = <<~MD
      # CLAUDE.md — base de conhecimento vbrain

      Este repositório é a **sua base de conhecimento pessoal vbrain** e é
      autossuficiente: contém os dados versionados (`raw/`, `wiki/`,
      `db/vbrain.sqlite3`, `config/`), as skills do agente em `.claude/skills/`
      e uma cópia do código Ruby que as skills usam (`scripts/`, `lib/`,
      `Gemfile`, `Gemfile.lock`).

      ## Regra principal — SEMPRE use as skills vbrain

      Toda operação na base passa pelas skills (slash commands). **Nunca** edite
      `wiki/`, `raw/` ou `db/` na mão, nem rode SQL direto: isso quebra o índice
      e o grafo de links.

      | Quero…                                          | Use a skill                       |
      |---|---|
      | Consultar a base                                | `/vbrain-query-knowledge`         |
      | Adicionar conhecimento (arquivo/URL/nota)       | `/vbrain-add-knowledge`           |
      | Conectar fonte realtime (calendar/gmail/slack)  | `/vbrain-add-realtime-knowledge`  |
      | Criar uma rotina                                | `/vbrain-add-routine`             |
      | Rodar as rotinas (watch loop)                   | `/vbrain-routine`                 |

      As skills (em `.claude/skills/`) usam caminhos relativos (`scripts/...`),
      então rode-as a partir da **raiz deste repo**.

      ## Pré-requisitos

      As skills são determinísticas em Ruby. Pra rodá-las aqui: **Ruby 3.3.6** e
      `bundle install` (o `Gemfile`/`Gemfile.lock` já estão neste repo). Não é
      preciso clonar o repo de código separado — o código está incluído aqui.

      ## Por quê (arquitetura)

      Wiki em markdown é a fonte da verdade; o SQLite (`db/vbrain.sqlite3`) é
      índice derivado — descartável (dá pra apagar e reconstruir com
      `scripts/reindex.rb`), mas versionado por conveniência. O LLM só entra pro
      que exige julgamento (chunkar, sintetizar páginas). O código aqui é uma
      cópia sincronizada por `scripts/install.rb` no repo de projeto; o canônico
      vive lá.
    MD

    # Escreve CLAUDE.md + copia skills + copia código. Retorna resumo pro JSON.
    def self.install!(dir = Paths.data_home, skills_src: SKILLS_SRC, src_root: SRC_ROOT)
      {
        "claude_md" => write_claude_md!(dir),
        "skills_installed" => install_skills!(dir, skills_src),
        "code_installed" => install_code!(dir, src_root)
      }
    end

    # Não clobbera um CLAUDE.md já existente (o usuário pode ter customizado).
    def self.write_claude_md!(dir = Paths.data_home)
      path = File.join(dir, "CLAUDE.md")
      return false if File.exist?(path)

      File.write(path, CLAUDE_MD)
      true
    end

    # Copia cada skill (subdiretório) de skills_src para <dir>/.claude/skills/.
    # Idempotente: remove o destino de cada skill antes de copiar.
    def self.install_skills!(dir = Paths.data_home, skills_src = SKILLS_SRC)
      return 0 unless Dir.exist?(skills_src)

      dest = File.join(dir, ".claude", "skills")
      FileUtils.mkdir_p(dest)
      names = Dir.children(skills_src).select { |c| File.directory?(File.join(skills_src, c)) }
      names.each do |name|
        target = File.join(dest, name)
        FileUtils.rm_rf(target)
        FileUtils.cp_r(File.join(skills_src, name), target)
      end
      names.length
    end

    # Copia o código (scripts/, lib/ e os arquivos de bundler/ruby) pra raiz da
    # base, tornando-a autossuficiente. Idempotente. Retorna nº de itens copiados.
    # Guard: nunca copia em cima de si mesmo (quando a base == repo de código).
    def self.install_code!(dir = Paths.data_home, src_root = SRC_ROOT)
      return 0 if File.expand_path(dir) == File.expand_path(src_root)

      copied = 0
      CODE_DIRS.each do |name|
        src = File.join(src_root, name)
        next unless Dir.exist?(src)

        target = File.join(dir, name)
        FileUtils.rm_rf(target)
        FileUtils.cp_r(src, target)
        copied += 1
      end
      CODE_FILES.each do |name|
        src = File.join(src_root, name)
        next unless File.exist?(src)

        FileUtils.cp(src, File.join(dir, name))
        copied += 1
      end
      copied
    end
  end
end
