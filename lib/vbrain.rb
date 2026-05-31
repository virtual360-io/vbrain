# A base é toda UTF-8 (wiki/ e raw/ em português). Não dependa do locale do
# ambiente: no cloud o LANG costuma vir vazio e o Ruby assume US-ASCII, o que
# quebra File.read/match de markdown com acento dentro das skills e rotinas.
# Fixa UTF-8 de forma determinística — código resolve o que código pode.
Encoding.default_external = Encoding::UTF_8
Encoding.default_internal = Encoding::UTF_8

module VBrain
end

require_relative "vbrain/paths"
require_relative "vbrain/db"
require_relative "vbrain/slug"
require_relative "vbrain/links"
require_relative "vbrain/page"
require_relative "vbrain/fts_query"
require_relative "vbrain/sources"
require_relative "vbrain/git"
require_relative "vbrain/scaffold"
require_relative "vbrain/realtime/gcalendar"
require_relative "vbrain/realtime/gmail"
require_relative "vbrain/realtime/slack"
require_relative "vbrain/routines"
