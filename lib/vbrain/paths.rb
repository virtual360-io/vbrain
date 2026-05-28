module VBrain
  module Paths
    ROOT     = File.expand_path("../..", __dir__).freeze
    RAW_DIR  = File.join(ROOT, "raw").freeze
    WIKI_DIR = File.join(ROOT, "wiki").freeze
    DB_DIR   = File.join(ROOT, "db").freeze
    DB_PATH  = File.join(DB_DIR, "vbrain.sqlite3").freeze
    TMP_DIR  = File.join(RAW_DIR, ".tmp").freeze

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

    def self.ensure_dirs!
      [RAW_DIR, WIKI_DIR, DB_DIR, TMP_DIR].each { |d| FileUtils.mkdir_p(d) }
      CATEGORIES.each { |c| FileUtils.mkdir_p(File.join(WIKI_DIR, c)) }
    end
  end
end

require "fileutils"
