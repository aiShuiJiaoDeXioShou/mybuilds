require 'spaceship'
require 'fastlane_core/itunes_transporter'
module MybuildsChannels
  module Apple
    # 真实fastlane JWT与无retry/redirect Faraday；不走APIClient自动刷新/重试。
    def self.request(token,method,path,body=nil,origin='https://api.appstoreconnect.apple.com')
      raise 'method_invalid' unless [:get,:post,:patch].include?(method)
      c=MybuildsChannels.connection(origin)
      response=c.run_request(method,path,body ? JSON.generate(body) : nil,{'Authorization'=>"Bearer #{token}",'Content-Type'=>'application/json'})
      raise 'remote_unconfirmed' unless response.status.between?(200,299)
      JSON.parse(response.body.to_s.empty? ? '{}' : response.body)
    end
    def self.token(job)
      k=JSON.parse(File.binread(job.fetch('credential')))
      Spaceship::ConnectAPI::Token.create(key_id:k.fetch('key_id'),issuer_id:k.fetch('issuer_id'),key:k.fetch('key'),duration:k.fetch('duration',200),in_house:false).text
    end
    def self.apps(token,bundle)
      r=request(token,:get,"/v1/apps?filter%5BbundleId%5D=#{bundle}&limit=2")
      rows=r.fetch('data');raise 'app_invalid' unless rows.length==1&&rows[0].dig('attributes','bundleId')==bundle
      rows[0].fetch('id')
    end
    def self.body(action,a)
      version=a['app_store_version_id'];build=a['build_id'];submission=a['review_submission_id']
      case action
      when 'select_build';[:patch,"/v1/appStoreVersions/#{version}/relationships/build",{'data'=>{'type'=>'builds','id'=>build}}]
      when 'set_release_policy';[:patch,"/v1/appStoreVersions/#{version}",{'data'=>{'type'=>'appStoreVersions','id'=>version,'attributes'=>{'releaseType'=>a['automatic_release'] ? 'AFTER_APPROVAL' : 'MANUAL'}}}]
      when 'create_review';[:post,'/v1/reviewSubmissions',{'data'=>{'type'=>'reviewSubmissions','attributes'=>{'platform'=>'IOS'},'relationships'=>{'app'=>{'data'=>{'type'=>'apps','id'=>a.fetch('app_id')}}}}}]
      when 'add_review_item';[:post,'/v1/reviewSubmissionItems',{'data'=>{'type'=>'reviewSubmissionItems','relationships'=>{'reviewSubmission'=>{'data'=>{'type'=>'reviewSubmissions','id'=>submission}},'appStoreVersion'=>{'data'=>{'type'=>'appStoreVersions','id'=>version}}}}}]
      when 'submit_review';[:patch,"/v1/reviewSubmissions/#{submission}",{'data'=>{'type'=>'reviewSubmissions','id'=>submission,'attributes'=>{'submitted'=>true}}}]
      else;raise 'action_invalid'
      end
    end
    def self.canonical(value)
      return value.keys.sort.to_h{|k|[k,canonical(value[k])]} if value.is_a?(Hash)
      return value.map{|v|canonical(v)} if value.is_a?(Array)
      value
    end
    # fastlane仅产生固定altool argv和本次临时p8；实际执行归Go process.Run。
    def self.transport_command(job)
      k=JSON.parse(File.binread(job.fetch('credential')))
      executor=FastlaneCore::AltoolTransporterExecutor.new
      prepared=executor.prepare(original_api_key:{key_id:k.fetch('key_id'),issuer_id:k.fetch('issuer_id'),key:k.fetch('key')})
      command=executor.build_upload_command(nil,nil,job.fetch('artifact'),{platform:'ios',api_key:prepared.merge(key_dir:Shellwords.escape(prepared.fetch(:key_dir)))})
      parts=Shellwords.split(command)
      raise 'transport_command_invalid' unless parts.shift=='API_PRIVATE_KEYS_DIR='+prepared.fetch(:key_dir)&&parts.shift=='xcrun'
      {'args'=>parts,'key_dir'=>prepared.fetch(:key_dir)}
    end
    def self.next_action(jwt,job,app,origin='https://api.appstoreconnect.apple.com')
      previous=job.fetch('previous');done=previous.to_h{|r|[r.fetch('mutation_stage'),r]}
      return nil if done.key?('submit_review')
      deadline=Process.clock_gettime(Process::CLOCK_MONOTONIC)+25
      builds=nil;build=nil
      loop do
        builds=request(jwt,:get,"/v1/builds?filter%5Bapp%5D=#{app}&filter%5Bversion%5D=#{job.fetch('number')}&include=preReleaseVersion&limit=2",nil,origin)
        rows=builds.fetch('data');raise 'build_conflict' if rows.length>1||builds.dig('links','next')
        build=rows.first
        break if build && build.dig('attributes','processingState')=='VALID'
        raise 'apple_processing_timeout' if build && ['FAILED','INVALID'].include?(build.dig('attributes','processingState')) || Process.clock_gettime(Process::CLOCK_MONOTONIC)>=deadline
        sleep 0.5
      end
      raise 'build_version_invalid' unless build['type']=='builds'&&build.dig('attributes','version')==job.fetch('number').to_s
      prerelease=build.dig('relationships','preReleaseVersion','data','id')
      versions=Array(builds['included']).select{|r|r['type']=='preReleaseVersions'&&r['id']==prerelease}
      raise 'build_version_invalid' unless versions.length==1&&versions[0].dig('attributes','version')==job.fetch('version_name')&&versions[0].dig('attributes','platform')=='IOS'
      response=request(jwt,:get,"/v1/apps/#{app}/appStoreVersions?filter%5Bplatform%5D=IOS&include=build&limit=50",nil,origin)
      raise 'pagination_limit' if response.dig('links','next')
      editing=response.fetch('data').select{|r|r.dig('attributes','versionString')==job['version_name']}
      raise 'version_missing' unless editing.length==1
      version=editing[0];state=version.dig('attributes','appStoreState')
      raise 'version_not_editable' unless ['PREPARE_FOR_SUBMISSION','DEVELOPER_REJECTED','REJECTED','METADATA_REJECTED','READY_FOR_REVIEW'].include?(state)
      a={'action'=>'','request_sha256'=>'','app_store_version_id'=>version['id'],'build_id'=>build['id'],'submit_for_review'=>job['submit'],'automatic_release'=>job['automatic']}
      selected=version.dig('relationships','build','data','id')
      if selected!=build['id']
        raise 'completed_action_changed' if done.key?('select_build')
        return a.merge('action'=>'select_build')
      end
      policy=job['automatic'] ? 'AFTER_APPROVAL' : 'MANUAL'
      if version.dig('attributes','releaseType')!=policy
        raise 'completed_action_changed' if done.key?('set_release_policy')
        return a.merge('action'=>'set_release_policy')
      end
      if !done.key?('create_review')
        reviews=request(jwt,:get,"/v1/reviewSubmissions?filter%5Bapp%5D=#{app}&filter%5Bplatform%5D=IOS&limit=16",nil,origin)
        raise 'foreign_review_exists' if reviews.dig('links','next')||reviews.fetch('data').any?{|r|!['COMPLETE','CANCELLED'].include?(r.dig('attributes','state'))}
        return a.merge('action'=>'create_review')
      end
      submission=done.fetch('create_review').dig('remote','apple','review_submission_id');raise 'submission_missing' unless submission
      review=request(jwt,:get,"/v1/reviewSubmissions/#{submission}?include=app",nil,origin).fetch('data')
      raise 'submission_invalid' unless review['id']==submission&&review.dig('relationships','app','data','id')==app&&review.dig('attributes','state')=='READY_FOR_REVIEW'
      a['review_submission_id']=submission
      if !done.key?('add_review_item')
        items=request(jwt,:get,"/v1/reviewSubmissions/#{submission}/items?limit=16",nil,origin)
        raise 'foreign_item_exists' unless items.fetch('data').empty?&&!items.dig('links','next')
        return a.merge('action'=>'add_review_item')
      end
      item=done.fetch('add_review_item').dig('remote','apple','review_item_id');raise 'item_missing' unless item
      members=request(jwt,:get,"/v1/reviewSubmissions/#{submission}/items?include=appStoreVersion&limit=16",nil,origin)
      raise 'item_invalid' if members.dig('links','next')
      entries=members.fetch('data')
      raise 'item_invalid' unless entries.length==1&&entries[0]['id']==item&&entries[0].dig('relationships','appStoreVersion','data','id')==version['id']
      a.merge('action'=>'submit_review','review_item_id'=>item)
    end
    def self.query(jwt,job,app,origin='https://api.appstoreconnect.apple.com')
        q=job.fetch('query');a=q['apple_authorization'];original=q['apple']||{}
        if !a || a['action']=='upload_binary'
          response=request(jwt,:get,"/v1/builds?filter%5Bapp%5D=#{app}&filter%5Bversion%5D=#{q.fetch('version_code')}&limit=16",nil,origin)
          rows=response.fetch('data');raise 'matches_limit' if rows.length>16||response.dig('links','next')
          matches=rows.map{|r|{'version_codes'=>[q['version_code']],'lifecycle'=>r.dig('attributes','processingState').to_s,'apple'=>{'app_id'=>app,'build_id'=>r.fetch('id'),'processing_state'=>r.dig('attributes','processingState').to_s,'action_confirmed'=>false}}}
          return {'status'=>'unknown','reason'=>'observation_insufficient','remote'=>{},'matches'=>matches}
        end
        # 只读取原精确对象；不足以关联原动作时保持unknown，不创建版本/提交。
        remote={'app_id'=>app,'request_sha256'=>a.fetch('request_sha256'),'action_confirmed'=>false};evidence=[]
        version=nil
        if a['app_store_version_id']
          v=request(jwt,:get,"/v1/appStoreVersions/#{a.fetch('app_store_version_id')}?include=app,build",nil,origin)
          version=v.fetch('data');raise 'version_mismatch' unless version['id']==a['app_store_version_id']&&version.dig('relationships','app','data','id')==app
          remote['app_store_version_id']=version['id'];remote['build_id']=version.dig('relationships','build','data','id');remote['release_type']=version.dig('attributes','releaseType');evidence<<v
        end
        case a.fetch('action')
        when 'select_build'
          raise 'build_mismatch' unless version&&remote['build_id']==a.fetch('build_id')
        when 'set_release_policy'
          raise 'release_mismatch' unless version&&remote['release_type']==(a['automatic_release'] ? 'AFTER_APPROVAL' : 'MANUAL')
        when 'create_review','add_review_item','submit_review'
          submission=a['review_submission_id']||original['review_submission_id'];raise 'submission_missing' unless submission
          if a['action']=='create_review'
            raise 'original_response_missing' unless original['request_sha256']==a['request_sha256']&&original['response_sha256'].to_s.match?(/\A[0-9a-f]{64}\z/)
          end
          response=request(jwt,:get,"/v1/reviewSubmissions/#{submission}?include=app",nil,origin);review=response.fetch('data')
          raise 'review_mismatch' unless review['id']==submission&&review.dig('relationships','app','data','id')==app
          remote['review_submission_id']=submission;remote['review_state']=review.dig('attributes','state').to_s;evidence<<response
          if a['action']!='create_review'
            item=a['review_item_id']||original['review_item_id'];raise 'item_missing' unless item
            if a['action']=='add_review_item'
              raise 'original_response_missing' unless original['request_sha256']==a['request_sha256']&&original['response_sha256'].to_s.match?(/\A[0-9a-f]{64}\z/)
            end
            members=request(jwt,:get,"/v1/reviewSubmissions/#{submission}/items?include=appStoreVersion&limit=16",nil,origin)
            entries=members.fetch('data');raise 'item_mismatch' unless !members.dig('links','next')&&entries.length==1&&entries[0]['id']==item&&entries[0].dig('relationships','appStoreVersion','data','id')==a.fetch('app_store_version_id')
            remote['review_item_id']=item;evidence<<members
            if a['action']=='submit_review'
              raise 'submitted_unconfirmed' unless version&&remote['build_id']==a.fetch('build_id')&&['WAITING_FOR_REVIEW','IN_REVIEW','COMPLETING','COMPLETE'].include?(remote['review_state'])&&review.dig('attributes','submittedDate').is_a?(String)
            end
          end
        else;raise 'action_unsupported'
        end
        remote['response_sha256']=Digest::SHA256.hexdigest(JSON.generate(canonical(evidence)));remote['action_confirmed']=true
        matches=[{'version_codes'=>[q.fetch('version_code')],'lifecycle'=>remote['review_state']||remote['release_type']||'confirmed','apple'=>remote}]
        return {'status'=>'confirmed','reason'=>'','remote'=>{'apple'=>remote},'matches'=>matches}
    end
    def self.run(job)
      jwt=token(job);app=apps(jwt,job.fetch('app'))
      return {'status'=>'ready','reason'=>'','remote'=>{'apple'=>{'app_id'=>app,'action_confirmed'=>false}},'matches'=>[]} if job['operation']=='apple_preflight'
      if job['operation']=='apple_transport_prepare'
        return {'status'=>'ready','reason'=>'','remote'=>{'apple'=>{'app_id'=>app}},'matches'=>[],'transport'=>transport_command(job)}
      end
      if job['operation']=='apple_next'
        begin
          action=next_action(jwt,job,app)
          return {'status'=>'ready','reason'=>'','remote'=>{},'matches'=>[],'next'=>action}.reject{|k,v|k=='next'&&v.nil?}
        rescue StandardError=>e
          reason=e.message=='apple_processing_timeout' ? 'apple_processing_timeout' : 'apple_precondition_failed'
          return {'status'=>'unknown','reason'=>reason,'remote'=>{},'matches'=>[]}
        end
      end
      return query(jwt,job,app) if job['operation']=='apple_query'
      g=job.fetch('grant');a=g.fetch('apple');action=a.fetch('action');remote={'app_id'=>app,'request_sha256'=>a.fetch('request_sha256'),'action_confirmed'=>false}
      if action=='upload_binary'
        raise 'transport_go_only'
      else
        raise 'submit_not_authorized' unless a['submit_for_review']
        raise 'automatic_not_authorized' if a['automatic_release']&&!a['submit_for_review']
        parameters=a.merge('app_id'=>app);method,path,payload=body(action,parameters)
        # 授权摘要绑定具体规范body，发送前再次核对。
        canonical=JSON.generate(canonical(payload));raise 'request_digest_mismatch' unless Digest::SHA256.hexdigest(canonical)==a.fetch('request_sha256')
        r=request(jwt,method,path,payload);data=r.fetch('data');remote['response_sha256']=Digest::SHA256.hexdigest(JSON.generate(r))
        case action
        when 'select_build';raise 'build_mismatch' unless data['id']==a['build_id'];remote['build_id']=data['id'];remote['app_store_version_id']=a['app_store_version_id']
        when 'set_release_policy';raise 'release_mismatch' unless data['id']==a['app_store_version_id']&&data.dig('attributes','releaseType')==(a['automatic_release'] ? 'AFTER_APPROVAL' : 'MANUAL');remote['app_store_version_id']=data['id'];remote['release_type']=data.dig('attributes','releaseType')
        when 'create_review';remote['review_submission_id']=data.fetch('id')
        when 'add_review_item';remote['review_item_id']=data.fetch('id');remote['review_submission_id']=a['review_submission_id'];remote['app_store_version_id']=a['app_store_version_id']
        when 'submit_review';raise 'review_unconfirmed' unless data['id']==a['review_submission_id']&&['WAITING_FOR_REVIEW','IN_REVIEW','COMPLETING','COMPLETE'].include?(data.dig('attributes','state'))&&data.dig('attributes','submittedDate').is_a?(String);remote['review_submission_id']=data['id'];remote['review_state']=data.dig('attributes','state').to_s;remote['app_store_version_id']=a.fetch('app_store_version_id');remote['build_id']=a.fetch('build_id');remote['review_item_id']=a.fetch('review_item_id')
        end
        remote['action_confirmed']=true
      end
      {'status'=>'confirmed','reason'=>'','remote'=>{'apple'=>remote},'matches'=>[]}
    end
  end
end
