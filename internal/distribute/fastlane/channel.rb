# 只允许本项目的具体入口；不读取用户lane、ADC或个人账户。
File.umask(077)
require 'json'
require 'digest'
require 'fastlane'
require 'faraday'
require 'faraday/net_http'
require_relative 'google_play'
require_relative 'apple'
{'fastlane'=>'2.240.1','google-apis-core'=>'1.2.5','faraday-net_http'=>'3.4.4'}.each do |name,version|
  raise 'tool_version_invalid' unless Gem.loaded_specs.fetch(name).version.to_s==version
end
module MybuildsChannels
  class ResponseLimit < Faraday::Middleware
    def call(env)
      body=+''
      env.request.on_data = proc { |chunk, total, _env| raise 'response_limit' if total > 1_048_576;body<<chunk.to_s }
      @app.call(env).on_complete{|response|response.body=body}
    end
  end
  def self.connection(origin = nil)
    Faraday.new(url: origin, request: {open_timeout: 10, timeout: 30}) do |f|
      f.use ResponseLimit
      f.adapter :net_http, max_retries: 0
    end
  end
  def self.execute
    job = JSON.parse(File.binread(ENV.fetch('MYBUILDS_PUBLISH_JOB')))
    result = {'status'=>'unknown','reason'=>'remote_unconfirmed','remote'=>{},'matches'=>[]}
    begin
      result = job.fetch('operation').start_with?('play_') ? GooglePlay.run(job) : Apple.run(job)
    rescue StandardError
      # 不把第三方异常、HTTPbody、JWT或材料路径放进结果或日志。
    end
    raw=JSON.generate(result)
    raise 'result_limit' if raw.bytesize>65536
    File.open(job.fetch('result'),File::WRONLY|File::CREAT|File::EXCL,0600){|f|f.write(raw);f.flush;f.fsync}
  end
end
