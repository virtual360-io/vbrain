require "fileutils"

module VBrain
  module Paths
    PROJECT_ROOT = File.expand_path("../..", __dir__).freeze

    # Páginas de conhecimento vivem na raiz de wiki/ (espaço plano, estilo
    # ai-memory). `_realtime` é o único subdir especial — páginas fantasma com
    # handler MCP, escritas por outra skill, não pelo pipeline de ingest.
    REALTIME_DIR = "_realtime".freeze

    # kind é metadado livre no frontmatter; não determina mais a pasta.
    KINDS = %w[concept decision gotcha note rule realtime].freeze

    def self.data_home
      env = ENV["VBRAIN_HOME"]
      return File.expand_path(env) if env && !env.empty?

      File.expand_path("~/vbrain")
    end

    def self.raw_dir;   File.join(data_home, "raw");   end
    def self.wiki_dir;  File.join(data_home, "wiki");  end
    def self.db_dir;    File.join(data_home, "db");    end
    def self.db_path;   File.join(db_dir, "vbrain.sqlite3"); end
    def self.tmp_dir;   File.join(raw_dir, ".tmp");    end

    def self.ensure_dirs!
      [raw_dir, wiki_dir, db_dir, tmp_dir].each { |d| FileUtils.mkdir_p(d) }
      FileUtils.mkdir_p(File.join(wiki_dir, REALTIME_DIR))
    end
  end
end
