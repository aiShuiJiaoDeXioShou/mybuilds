require 'supply'
require 'google/apis/androidpublisher_v3'
require 'googleauth'
module MybuildsChannels
  module GooglePlay
    def self.service(token, origin = nil)
      s=Google::Apis::AndroidpublisherV3::AndroidPublisherService.new
      s.authorization=token # 普通短token字符串，写请求无refreshable authorization。
      s.request_options.retries=0
      s.client=MybuildsChannels.connection
      if origin # 仅实际库的本机测试调用；生产job从不接受origin。
        s.root_url=origin
      end
      s
    end
    def self.token(job)
      if File.file?(job.fetch('token'))
        saved=JSON.parse(File.binread(job['token']))
        raise 'token_expired' if saved.fetch('expires_at').to_i<=Time.now.to_i+30
        return saved.fetch('access_token')
      end
      credential=Google::Auth::ServiceAccountCredentials.make_creds(json_key_io: StringIO.new(File.binread(job.fetch('credential'))),scope:'https://www.googleapis.com/auth/androidpublisher')
      credential.fetch_access_token!
      saved={'access_token'=>credential.access_token,'expires_at'=>credential.expires_at.to_i}
      File.open(job.fetch('token'),File::WRONLY|File::CREAT|File::EXCL,0600){|f|f.write(JSON.generate(saved));f.flush;f.fsync}
      saved['access_token']
    end
    def self.query(s, job)
      q=job.fetch('query');track=q['track'].to_s.empty? ? 'internal' : q['track']
      result=s.list_application_track_releases("applications/#{job.fetch('app')}/tracks/#{track}")
      rows=Array(result.releases);raise 'matches_limit' if rows.length>20
      rows=[] if q['kind']=='doctor'
      rows=rows.select{|r|Array(r.active_artifacts).any?{|a|a.version_code.to_i==q['version_code'].to_i}} unless q['kind']=='doctor'
      raise 'matches_limit' if rows.length>16
      matches=rows.map do |r|
        codes=Array(r.active_artifacts).map{|artifact|artifact.version_code.to_i};raise 'codes_limit' if codes.length>100
        {'release_name'=>r.release_name.to_s,'track'=>r.track.to_s,'version_codes'=>codes,'lifecycle'=>r.release_lifecycle_state.to_s}
      end
      {'status'=>'unknown','reason'=>'observation_insufficient','remote'=>{},'matches'=>matches}
    end
    def self.run(job)
      s=service(token(job))
      if job.fetch('operation')=='play_preflight'
        if job['query'] && job['query']['kind']=='doctor'
          query(s,job)
        end
        return {'status'=>'ready','reason'=>'','remote'=>{},'matches'=>[]}
      end
      return query(s,job) if job.fetch('operation')=='play_query'
      g=job.fetch('grant');app=g.fetch('app_identifier');code=g.fetch('version_code');stage='insert_edit'
      edit=s.insert_edit(app);raise 'edit_invalid' unless edit.id.to_s.match?(/\A[A-Za-z0-9_-]{1,128}\z/)
      versions=Array(s.list_edit_bundles(app,edit.id).bundles).map(&:version_code)+Array(s.list_edit_apks(app,edit.id).apks).map(&:version_code)
      raise 'version_conflict' if versions.include?(code)
      tracks=Array(s.list_edit_tracks(app,edit.id).tracks);raise 'track_missing' unless tracks.any?{|t|t.track==g.fetch('track')}
      stage='upload_bundle';bundle=s.upload_edit_bundle(app,edit.id,upload_source:job.fetch('artifact'),content_type:'application/octet-stream')
      raise 'bundle_mismatch' unless bundle.version_code==code&&bundle.sha256.to_s.downcase==g.fetch('artifact_sha256')
      release=Google::Apis::AndroidpublisherV3::TrackRelease.new(name:g.fetch('release_name'),version_codes:[code],status:g.fetch('release_status'))
      track=Google::Apis::AndroidpublisherV3::Track.new(track:g.fetch('track'),releases:[release])
      stage='update_track';accepted=s.update_edit_track(app,edit.id,g['track'],track)
      ar=Array(accepted.releases);raise 'track_mismatch' unless accepted.track==g['track']&&ar.length==1&&ar[0].name==g['release_name']&&ar[0].status==g['release_status']&&Array(ar[0].version_codes)==[code]
      stage='commit';committed=s.commit_edit(app,edit.id,changes_not_sent_for_review:g.fetch('changes_not_sent_for_review',false))
      raise 'commit_mismatch' unless committed.id==edit.id
      {'status'=>'confirmed','reason'=>'','remote'=>{'edit_id'=>edit.id,'release_name'=>g['release_name'],'track'=>g['track'],'bundle_sha256'=>g['artifact_sha256'],'version_code'=>code,'lifecycle'=>'uploaded','bundle_accepted'=>true,'track_accepted'=>true,'commit_accepted'=>true},'matches'=>[]}
    end
  end
end
