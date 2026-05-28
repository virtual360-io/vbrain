#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "digest"
require "fileutils"
require "optparse"
require "time"
require "vbrain"

opts = { force: false, type: nil }
parser = OptionParser.new do |o|
  o.banner = "Usage: ingest_raw.rb <path> [--type TYPE] [--force]"
  o.on("--type TYPE", "Force source type")  { |v| opts[:type] = v }
  o.on("--force", "Ingest even if sha256 duplicate") { opts[:force] = true }
end
parser.parse!(ARGV)

path = ARGV.shift
abort(parser.help) if path.nil? || path.empty?
abort("path not found: #{path}") unless File.exist?(path)

VBrain::Paths.ensure_dirs!

source = if opts[:type]
           VBrain::Sources.for(opts[:type])
         else
           VBrain::Sources.detect(path)
         end

if source.nil?
  ext = File.extname(path)
  sniff = File.file?(path) ? File.binread(path, 64).inspect : "(directory)"
  puts JSON.generate("source_type" => "unknown", "ext" => ext, "sniff" => sniff, "path" => path)
  exit 0
end

sha = Digest::SHA256.file(path).hexdigest

VBrain::DB.open do |db|
  existing = db.execute("SELECT id, path FROM raw_sources WHERE sha256 = ?", [sha]).first
  if existing && !opts[:force]
    warn "duplicate sha256, raw already at #{existing['path']} (use --force to override)"
    puts JSON.generate(
      "raw_id" => existing["id"],
      "raw_path" => existing["path"],
      "source_type" => source.kind_key,
      "duplicate" => true
    )
    exit 0
  end

  basename = File.basename(path)
  stamp = Time.now.utc.strftime("%Y%m%dT%H%M%SZ")
  dest = File.join(VBrain::Paths::RAW_DIR, "#{stamp}-#{basename}")
  FileUtils.cp(path, dest)

  db.execute(
    "INSERT INTO raw_sources (path, original_filename, source_type, sha256) VALUES (?, ?, ?, ?)",
    [dest, basename, source.kind_key, sha]
  )
  raw_id = db.last_insert_row_id

  out_path = File.join(VBrain::Paths::TMP_DIR, "extracted-#{raw_id}.txt")
  source.extract(path, out_path)

  puts JSON.generate(
    "raw_id" => raw_id,
    "raw_path" => dest,
    "source_type" => source.kind_key,
    "extracted_path" => out_path
  )
end
