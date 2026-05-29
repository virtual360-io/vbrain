module VBrain
  module FtsQuery
    STOP_CHARS = /[":()\[\]{}<>!?,;`]/

    # Stopwords PT-BR (+ algumas EN comuns): palavras-função, pronomes,
    # interrogativas e verbos auxiliares de alta frequência. Em consultas em
    # linguagem natural ("quais empregos eu já tive") elas casam com quase
    # tudo e, sob OR, afogam o sinal no BM25. Filtramos antes de montar a
    # query. Comparação é case-insensitive; incluímos formas com e sem acento
    # porque o token preserva o acento original.
    STOPWORDS = %w[
      a o as os um uma uns umas
      de do da dos das em no na nos nas ao aos à às
      por pra para per com sem sob sobre entre ate até desde
      e ou mas nem que se como quando onde porque pois
      qual quais quanto quanta quantos quantas quem cujo cuja
      eu tu ele ela nos vos eles elas voce voces vc vcs
      me te lhe nos vos lhes meu minha meus minhas teu tua seu sua seus suas
      este esta isto esse essa isso aquele aquela aquilo
      ja já nao não sim talvez muito muita pouco pouca mais menos
      ter tem tenho tinha tive teve tinham foram foi sou somos sao são
      era eram estar esta está estou estava estavam ser
      the of to in on for and or is are was were be been being
      i you he she it we they my your what which who when where how
    ].freeze

    STOPWORD_SET = STOPWORDS.to_h { |w| [w, true] }.freeze

    def self.normalize(query, prefix: false)
      return "" if query.nil?

      cleaned = query.to_s.gsub(STOP_CHARS, " ")
      tokens = cleaned.split(/\s+/).reject(&:empty?)
      return "" if tokens.empty?

      kept = tokens.reject { |t| STOPWORD_SET[t.downcase] }
      # Se só sobraram stopwords (ex.: "quem é você"), não devolve vazio —
      # cair pros tokens originais é melhor que zero resultado.
      tokens = kept unless kept.empty?

      tokens = tokens.map { |t| prefix ? %("#{t}"*) : %("#{t}") }
      tokens.join(" OR ")
    end
  end
end
