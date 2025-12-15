#!/usr/bin/env fish

function psurf
    set app_path "/usr/bin/pathsurfer"
    
    if not test -x "$app_path"
        echo "error: pathsurfer not found or isn't executable at $app_path"
        return 1
    end

    set target_dir ("$app_path" $argv)

    if test -n "$target_dir"; and test -d "$target_dir"
        cd "$target_dir"
    end
end

