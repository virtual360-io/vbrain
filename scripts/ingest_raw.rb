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
  o.banner = "Usage: ingest_raw.rb <path-or-url> [--type TYPE] [--force]"
  o.on("--type TYPE") { |v| opts[:type] = v }
  o.on("--force")     { opts[:force] = true }
end
parser.parse!(ARGV)

input = ARGV.shift
abort(parser.help) if input.nil? || input.empty?

is_url = input.match?(%r{\Ahttps?://}i)
abort("path not found: #{input}") if !is_url && !File.exist?(input)

VBrain::Paths.ensure_dirs!

source = if opts[:type]
           VBrain::Sources.for(opts[:type])
         else
           VBrain::Sources.detect(input)
         end

if source.nil?
  ext = File.extname(input.to_s)
  sniff = File.file?(input) ? File.binread(input, 64).inspect : "(not a file)"
  puts JSON.generate("source_type" => "unknown", "ext" => ext, "sniff" => sniff, "input" => input)
  exit 0
end

timestamp = Time.now.utc.strftime("%Y%m%dT%H%M%SZ")

begin
  raw_info = source.copy_to_raw(input, VBrain::Paths.raw_dir, timestamp)
rescue StandardError => e
  abort("source #{source.kind_key} failed to ingest: #{e.class}: #{e.message}")
end

VBrain::DB.open do |db|
  existing = db.execute("SELECT id, path FROM raw_sources WHERE sha256 = ?", [raw_info["sha256"]]).first
  if existing && !opts[:force]
    warn "duplicate sha256, raw already at #{existing['path']} (use --force to override)"
    File.delete(raw_info["path"]) if File.exist?(raw_info["path"]) && raw_info["path"] != existing["path"]
    puts JSON.generate(
      "raw_id" => existing["id"],
      "raw_path" => existing["path"],
      "source_type" => source.kind_key,
      "duplicate" => true
    )
    exit 0
  end

  db.execute(
    "INSERT INTO raw_sources (path, original_filename, source_type, sha256) VALUES (?, ?, ?, ?)",
    [raw_info["path"], raw_info["original_filename"], source.kind_key, raw_info["sha256"]]
  )
  raw_id = db.last_insert_row_id

  out_path = File.join(VBrain::Paths.tmp_dir, "extracted-#{raw_id}.txt")
  source.extract(input, out_path, raw_info: raw_info)

  puts JSON.generate(
    "raw_id" => raw_id,
    "raw_path" => raw_info["path"],
    "source_type" => source.kind_key,
    "extracted_path" => out_path
  )
end
