import io.killedkenny.crossfuzz.Crossfuzz;
import org.json.JSONObject;
import org.json.JSONArray;
import org.json.JSONException;

public class JavaJsonTarget implements Crossfuzz.FuzzTarget {

    @Override
    public byte[] fuzz(byte[] input) throws Exception {
        String s = new String(input, java.nio.charset.StandardCharsets.UTF_8);
        try {
            JSONObject obj = new JSONObject(s);
            StringBuilder sb = new StringBuilder();
            if (obj.has("type")) {
                sb.append(obj.getString("type"));
            }
            if (obj.has("value")) {
                Object v = obj.get("value");
                if (v instanceof JSONArray arr) {
                    sb.append(arr.length());
                } else {
                    sb.append(v.toString());
                }
            }
            if (obj.has("nested")) {
                JSONObject nested = obj.getJSONObject("nested");
                sb.append(nested.length());
            }
            return sb.toString().getBytes();
        } catch (JSONException e) {
            return new byte[0];
        }
    }

    public static void main(String[] args) throws Exception {
        Crossfuzz.fuzz(new JavaJsonTarget());
    }
}
