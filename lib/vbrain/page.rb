require "yaml"
require "digest"
require "date"
require "fileutils"

module VBrain
  module Page
    FRONTMATTER_RE = /\A---\s*\n(.*?)\n---\s*\n(.*)\z/m

    class Error < StandardError; end

    Parsed = Struct.new(:frontmatter, :body, :sha256, keyword_init: true)

    def self.parse(path)
      content = File.read(path)
      parse_string(content)
    end

    def self.parse_string(content)
      if (m = content.match(FRONTMATTER_RE))
        fm = YAML.safe_load(m[1], permitted_classes: [Symbol, Date, Time]) || {}
        body = m[2]
      else
        fm = {}
        body = content
      end
      Parsed.new(frontmatter: fm, body: body, sha256: Digest::SHA256.hexdigest(body))
    end

    def self.write(dir:, slug:, frontmatter:, body:)
      raise Error, "dir must exist" unless Dir.exist?(dir)
      raise Error, "slug cannot be empty" if slug.nil? || slug.empty?

      full = File.join(dir, "#{slug}.md")
      content = render(frontmatter, body)

      tmp = "#{full}.tmp.#{Process.pid}.#{rand(1 << 32)}"
      File.write(tmp, content)
      File.rename(tmp, full)

      full
    end

    # Reescreve só o CORPO de uma página, preservando o frontmatter verbatim
    # (sem reserializar YAML — zero churn de formatação/ordem de chaves).
    # Passa o body atual ao bloco; grava de volta atomicamente só se mudou.
    # Retorna true se reescreveu, false se o bloco devolveu o body inalterado.
    def self.rewrite_body!(path)
      content = File.read(path)
      m = content.match(FRONTMATTER_RE)
      body = m ? m[2] : content
      new_body = yield(body)
      return false if new_body == body

      new_content = m ? "---\n#{m[1]}\n---\n#{new_body}" : new_body
      tmp = "#{path}.tmp.#{Process.pid}.#{rand(1 << 32)}"
      File.write(tmp, new_content)
      File.rename(tmp, path)
      true
    end

    def self.render(frontmatter, body)
      "#{YAML.dump(stringify_keys(frontmatter))}---\n#{body}"
    end

    def self.stringify_keys(hash)
      hash.each_with_object({}) do |(k, v), out|
        out[k.to_s] = v
      end
    end
  end
end
