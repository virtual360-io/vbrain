require "fileutils"

module VBrain
  module Paths
    PROJECT_ROOT = File.expand_path("../..", __dir__).freeze

    CATEGORIES = %w[concepts decisions gotchas notes _rules].freeze
    KINDS      = %w[concept decision gotcha note rule].freeze

    CATEGORY_TO_KIND = {
      "concepts"  => "concept",
      "decisions" => "decision",
      "gotchas"   => "gotcha",
      "notes"     => "note",
      "_rules"    => "rule"
    }.freeze

    KIND_TO_CATEGORY = CATEGORY_TO_KIND.invert.freeze

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
      CATEGORIES.each { |c| FileUtils.mkdir_p(File.join(wiki_dir, c)) }
    end
  end
end
