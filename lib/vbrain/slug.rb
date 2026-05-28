module VBrain
  module Slug
    MAX_LENGTH = 80

    class Error < StandardError; end

    def self.from(title, max_length: MAX_LENGTH)
      raise Error, "title cannot be nil or empty" if title.nil? || title.strip.empty?

      ascii = title.to_s
                   .unicode_normalize(:nfkd)
                   .encode("ASCII", undef: :replace, invalid: :replace, replace: "")
      ascii = ascii.downcase
      ascii = ascii.gsub(/[^a-z0-9]+/, "-")
      ascii = ascii.gsub(/-+/, "-")
      ascii = ascii.gsub(/\A-+|-+\z/, "")

      raise Error, "title yielded empty slug: #{title.inspect}" if ascii.empty?

      ascii[0, max_length].gsub(/-+\z/, "")
    end
  end
end
