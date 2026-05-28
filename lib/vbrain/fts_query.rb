module VBrain
  module FtsQuery
    STOP_CHARS = /[":()\[\]{}<>!?,;`]/

    def self.normalize(query, prefix: false)
      return "" if query.nil?

      cleaned = query.to_s.gsub(STOP_CHARS, " ")
      tokens = cleaned.split(/\s+/).reject(&:empty?)
      return "" if tokens.empty?

      tokens = tokens.map { |t| prefix ? %("#{t}"*) : %("#{t}") }
      tokens.join(" OR ")
    end
  end
end
